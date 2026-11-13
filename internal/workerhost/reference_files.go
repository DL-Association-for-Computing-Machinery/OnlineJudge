package workerhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"strconv"
	"time"
)

// UploadFile is an explicit P4 capability, separate from RoundTrip's M0 subset.
// The owner must validate the returned object by readback, maintain its cache
// ledger, and stop this exclusive host even after an unknown upload receipt.
func (h *Host) UploadFile(parent context.Context, data []byte) (string, error) {
	ctx, done, err := h.fileContext(parent, int64(len(data)))
	if err != nil {
		return "", err
	}
	defer done()
	var framing bytes.Buffer
	w := multipart.NewWriter(&framing)
	if _, err := w.CreateFormFile("file", "material"); err != nil {
		return "", errors.New("file framing failed")
	}
	at := framing.Len()
	if err := w.Close(); err != nil {
		return "", errors.New("file framing failed")
	}
	// Only headers and the final boundary are buffered. The finite private bytes
	// are sent through curl's upload stream, not its whole-body data option.
	body := io.MultiReader(bytes.NewReader(framing.Bytes()[:at]), bytes.NewReader(data), bytes.NewReader(framing.Bytes()[at:]))
	args := h.fileCurl("POST", "/file", 4096)
	args = append(args[:len(args)-1], "--header", "Content-Type: "+w.FormDataContentType(), "--upload-file", "-", args[len(args)-1])
	var out bytes.Buffer
	if err := h.fileExchange(ctx, body, &out, 4096, args); err != nil {
		return "", err
	}
	var id string
	if json.Unmarshal(out.Bytes(), &id) != nil {
		return "", fileError("file upload receipt unavailable", "decode")
	}
	if !filePath.MatchString("/file/" + id) {
		return "", fileError("file upload receipt unavailable", "ownership")
	}
	return id, nil
}

// ReadFile returns complete raw bytes only after a successful bounded transfer.
// The trusted execution adapter must reject IDs outside its own role ledger.
func (h *Host) ReadFile(parent context.Context, id string, maximum int64) ([]byte, error) {
	if !filePath.MatchString("/file/" + id) {
		return nil, fileError("invalid cached file identity", "ownership")
	}
	ctx, done, err := h.fileContext(parent, maximum)
	if err != nil {
		return nil, err
	}
	defer done()
	var out bytes.Buffer
	if err := h.fileExchange(ctx, nil, &out, maximum, h.fileCurl("GET", "/file/"+id, maximum)); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (h *Host) fileContext(parent context.Context, maximum int64) (context.Context, func(), error) {
	_, p, configErr := h.configuration()
	if configErr != nil {
		return nil, nil, configErr
	}
	if parent == nil || maximum < 0 || maximum > p.FileBytes {
		return nil, nil, fileError("reference file budget rejected", "budget")
	}
	if !h.fileBusy.CompareAndSwap(false, true) {
		return nil, nil, fileError("reference file transfer occupied", "ownership")
	}
	ctx, finish, err := h.begin(parent)
	if err != nil {
		h.fileBusy.Store(false)
		return nil, nil, err
	}
	h.mu.Lock()
	if h.fileOps >= p.Operations || maximum > p.PayloadBytes-h.filePayload {
		err = fileError("reference file operation budget exhausted", "budget")
	} else {
		h.fileOps++
		h.filePayload += maximum
	}
	h.mu.Unlock()
	if err != nil {
		finish()
		h.fileBusy.Store(false)
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	return ctx, func() { cancel(); finish(); h.fileBusy.Store(false) }, nil
}

func (h *Host) fileCurl(method, path string, maximum int64) []string {
	return []string{"exec", "-i", h.id, "curl", "--disable", "--silent", "--show-error", "--noproxy", "*", "--max-time", "20", "--max-filesize", strconv.FormatInt(maximum, 10), "--proto", "=http", "--request", method, "--write-out", "\n%{http_code}", "http://127.0.0.1:5050" + path}
}

func (h *Host) fileExchange(ctx context.Context, input io.Reader, output io.Writer, maximum int64, args []string) error {
	p, _, err := h.configuration()
	if err != nil {
		return err
	}
	s, err := h.inspect(ctx, h.id)
	if err != nil {
		return fileError("reference file host inspection failed", failureCode(err))
	}
	if err := h.validate(s, p); err != nil {
		return err
	}
	run := h.stream
	if run == nil {
		run = runCommandTo
		if h.nativeUbuntu() {
			run = ubuntuCommandTo
		}
	}
	response := &fileResponse{target: &commandSink{target: output, maximum: maximum}}
	name, commandArgs := h.dockerCommand(args)
	err = run(ctx, input, response, maximum+4, name, commandArgs...)
	if err != nil || ctx.Err() != nil {
		cause := errors.Join(err, ctx.Err())
		return errors.Join(fileError("reference file transfer failed", failureCode(cause)), contextCause(cause))
	}
	if !response.accepted() {
		return fileError("reference file HTTP response rejected", "http_rejected")
	}
	return nil
}

// Keep only curl's last four trailer bytes. Binary body newlines (including a
// body ending in the same trailer) remain ordinary body bytes, never delimiters.
type fileResponse struct {
	target io.Writer
	tail   [4]byte
	n      int
}

func (r *fileResponse) Write(p []byte) (int, error) {
	if r.n+len(p) <= 4 {
		copy(r.tail[r.n:], p)
		r.n += len(p)
		return len(p), nil
	}
	emit := r.n + len(p) - 4
	old := min(emit, r.n)
	if _, err := r.target.Write(r.tail[:old]); err != nil {
		return 0, err
	}
	fromP := emit - old
	if _, err := r.target.Write(p[:fromP]); err != nil {
		return 0, err
	}
	var next [4]byte
	n := copy(next[:], r.tail[old:r.n])
	copy(next[n:], p[fromP:])
	r.tail, r.n = next, 4
	return len(p), nil
}

func (r *fileResponse) accepted() bool { return r.n == 4 && r.tail == [4]byte{'\n', '2', '0', '0'} }

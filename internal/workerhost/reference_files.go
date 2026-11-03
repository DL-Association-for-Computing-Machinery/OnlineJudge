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

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
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
	if json.Unmarshal(out.Bytes(), &id) != nil || !filePath.MatchString("/file/"+id) {
		return "", errors.New("file upload receipt unavailable")
	}
	return id, nil
}

// ReadFile returns complete raw bytes only after a successful bounded transfer.
// The trusted execution adapter must reject IDs outside its own role ledger.
func (h *Host) ReadFile(parent context.Context, id string, maximum int64) ([]byte, error) {
	if !filePath.MatchString("/file/" + id) {
		return nil, errors.New("invalid cached file identity")
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
	p := experiment.PinnedReferenceFiles()
	if parent == nil || maximum < 0 || maximum > p.FileBytes {
		return nil, nil, errors.New("reference file budget rejected")
	}
	if !h.fileBusy.CompareAndSwap(false, true) {
		return nil, nil, errors.New("reference file transfer occupied")
	}
	ctx, finish, err := h.begin(parent)
	if err != nil {
		h.fileBusy.Store(false)
		return nil, nil, err
	}
	h.mu.Lock()
	if h.fileOps >= p.Operations || maximum > p.PayloadBytes-h.filePayload {
		err = errors.New("reference file operation budget exhausted")
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
	s, err := h.inspect(ctx, h.id)
	if err != nil {
		return errors.New("reference file host inspection failed")
	}
	if err := h.validate(s, experiment.PinnedEnvironment()); err != nil {
		return err
	}
	run := h.stream
	if run == nil {
		run = runCommandTo
	}
	response := &fileResponse{target: &commandSink{target: output, maximum: maximum}}
	err = run(ctx, input, response, maximum+4, "docker", append([]string{"--context", "desktop-linux"}, args...)...)
	if err != nil || ctx.Err() != nil {
		return errors.Join(errors.New("reference file transfer failed"), contextCause(errors.Join(err, ctx.Err())))
	}
	if !response.accepted() {
		return errors.New("reference file HTTP response rejected")
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

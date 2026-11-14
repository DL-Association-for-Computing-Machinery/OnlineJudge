package workerhost

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestNewFileAttemptRefreshesOnlyIdleSandboxBudget(t *testing.T) {
	h := &Host{tier: experiment.ReferenceAPlusBUbuntu, python3: true, active: make(map[uint64]context.CancelFunc)}
	_, profile, err := h.configuration()
	if err != nil {
		t.Fatal(err)
	}
	h.fileOps, h.filePayload = profile.Operations, profile.PayloadBytes
	if _, _, err := h.fileContext(context.Background(), 1); err == nil {
		t.Fatal("exhausted attempt could still transfer")
	}
	h.active[1] = func() {}
	if err := h.ResetFileAttemptBudget(); err == nil || h.fileOps != profile.Operations || h.filePayload != profile.PayloadBytes {
		t.Fatal("active sandbox unexpectedly refreshed")
	}
	delete(h.active, 1)
	h.fileBusy.Store(true)
	err = h.ResetFileAttemptBudget()
	h.fileBusy.Store(false)
	if err == nil || h.fileOps != profile.Operations || h.filePayload != profile.PayloadBytes {
		t.Fatal("file transfer unexpectedly refreshed its sandbox")
	}
	h.closed = true
	err = h.ResetFileAttemptBudget()
	h.closed = false
	if err == nil || h.fileOps != profile.Operations || h.filePayload != profile.PayloadBytes {
		t.Fatal("closed sandbox unexpectedly refreshed")
	}
	if err := h.ResetFileAttemptBudget(); err != nil {
		t.Fatal(err)
	}
	_, done, err := h.fileContext(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if h.fileOps != 1 || h.filePayload != 1 {
		t.Fatal("new attempt did not account for its first transfer")
	}
}

func TestReferenceFileFramingAndRawResponses(t *testing.T) {
	h, _ := opened(t)
	data := append(bytes.Repeat([]byte{0, 255, '\n', '2', '0', '0'}, 90000), []byte("\n200")...)
	h.stream = func(ctx context.Context, input io.Reader, out io.Writer, maximum int64, name string, args ...string) error {
		if name != "docker" || !strings.Contains(strings.Join(args, " "), "--context desktop-linux exec -i "+h.id+" curl --disable") {
			t.Fatal("unbound file dispatch")
		}
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 25*time.Second {
			t.Fatal("unbounded file command")
		}
		if input != nil {
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "--upload-file -") || strings.Contains(joined, "--data-binary") {
				t.Fatal("upload did not use a stream")
			}
			var contentType string
			for i, arg := range args {
				if arg == "--header" {
					contentType = strings.TrimPrefix(args[i+1], "Content-Type: ")
				}
			}
			kind, params, err := mime.ParseMediaType(contentType)
			if err != nil || kind != "multipart/form-data" {
				t.Fatal("bad MIME type")
			}
			reader := multipart.NewReader(input, params["boundary"])
			part, err := reader.NextPart()
			if err != nil || part.FormName() != "file" || part.FileName() != "material" {
				t.Fatal("bad file part")
			}
			raw, err := io.ReadAll(part)
			if err != nil || !bytes.Equal(raw, data) {
				t.Fatal("upload changed raw bytes")
			}
			if _, err := reader.NextPart(); err != io.EOF {
				t.Fatal("unexpected file part")
			}
			_, err = io.WriteString(out, "\"owned\"\n200")
			return err
		}
		if maximum != int64(len(data))+4 {
			t.Fatal("trailer charged as body")
		}
		for _, chunk := range [][]byte{data[:3], data[3:], []byte("\n2"), []byte("00")} {
			if _, err := out.Write(chunk); err != nil {
				return err
			}
		}
		return nil
	}
	id, err := h.UploadFile(context.Background(), data)
	if err != nil || id != "owned" {
		t.Fatal("upload failed", err)
	}
	raw, err := h.ReadFile(context.Background(), id, int64(len(data)))
	if err != nil || !bytes.Equal(raw, data) || len(h.active) != 0 {
		t.Fatal("download changed data or leaked active registration", err)
	}
}

func TestReferenceFileRejectsIncompleteOrOversizedTransfers(t *testing.T) {
	for _, response := range []string{"", "\n20", "body\n404", "oversize\n200", "body\n200extra"} {
		t.Run(response, func(t *testing.T) {
			h, _ := opened(t)
			h.stream = func(_ context.Context, _ io.Reader, out io.Writer, _ int64, _ string, _ ...string) error {
				_, err := io.WriteString(out, response)
				return err
			}
			raw, err := h.ReadFile(context.Background(), "owned", 4)
			if err == nil || raw != nil {
				t.Fatal("partial body accepted")
			}
		})
	}
	h, f := opened(t)
	before := len(f.calls)
	if _, err := h.ReadFile(context.Background(), "../foreign", 1); err == nil || len(f.calls) != before {
		t.Fatal("invalid file path dispatched")
	}
	if _, err := h.ReadFile(context.Background(), "owned", experiment.PinnedReferenceFiles().FileBytes+1); err == nil || len(f.calls) != before {
		t.Fatal("oversized file dispatched")
	}
	h.fileOps = experiment.PinnedReferenceFiles().Operations
	if _, err := h.UploadFile(context.Background(), nil); err == nil || len(f.calls) != before {
		t.Fatal("empty uploads escaped operation budget")
	}
}

func TestReferenceFileIOStaysRegisteredUntilStop(t *testing.T) {
	h, _ := opened(t)
	entered := make(chan struct{})
	h.stream = func(ctx context.Context, _ io.Reader, _ io.Writer, _ int64, _ string, _ ...string) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}
	finished := make(chan error, 1)
	go func() { _, err := h.ReadFile(context.Background(), "owned", 16); finished <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("file IO did not start")
	}
	if _, err := h.ReadFile(context.Background(), "other", 0); err == nil {
		t.Fatal("concurrent file IO admitted")
	}
	if err := h.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err == nil {
		t.Fatal("canceled file IO succeeded")
	}
	if _, err := h.UploadFile(context.Background(), nil); err == nil {
		t.Fatal("file IO reused fenced host")
	}
}

func TestStreamCommandBoundPreservesDefaultBudget(t *testing.T) {
	var out bytes.Buffer
	if err := runCommandTo(context.Background(), nil, &out, 512<<10, "/bin/sh", "-c", "head -c 524288 /dev/zero"); err != nil || out.Len() != 512<<10 {
		t.Fatal("bounded raw command failed", err)
	}
	if _, err := runCommand(context.Background(), nil, "/bin/sh", "-c", "head -c 131074 /dev/zero"); err == nil {
		t.Fatal("M0 command budget expanded")
	}
}

func TestReferenceEmptyFileDoesNotDisableBodyLimit(t *testing.T) {
	h, _ := opened(t)
	h.stream = func(_ context.Context, _ io.Reader, out io.Writer, maximum int64, _ string, _ ...string) error {
		if maximum != 4 {
			t.Fatal("wrong empty wire budget")
		}
		_, err := io.WriteString(out, "\n200")
		return err
	}
	if raw, err := h.ReadFile(context.Background(), "owned", 0); err != nil || len(raw) != 0 {
		t.Fatal("empty file rejected", err)
	}
	h.stream = func(_ context.Context, _ io.Reader, out io.Writer, _ int64, _ string, _ ...string) error {
		_, err := io.WriteString(out, "x\n200")
		return err
	}
	if _, err := h.ReadFile(context.Background(), "owned", 0); err == nil {
		t.Fatal("zero curl filesize disabled application limit")
	}
}

type readyFileWriter struct {
	target io.Writer
	ready  chan struct{}
	once   sync.Once
}

func (w *readyFileWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.ready) })
	return w.target.Write(p)
}

func TestFileStopDrainsRealSubprocessAndInputPipe(t *testing.T) {
	h, _ := opened(t)
	ready := make(chan struct{})
	h.stream = func(ctx context.Context, input io.Reader, out io.Writer, maximum int64, _ string, _ ...string) error {
		return runCommandTo(ctx, input, &readyFileWriter{target: out, ready: ready}, maximum, "/bin/sh", "-c", "printf READY; exec sleep 30")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := h.UploadFile(ctx, make([]byte, 1<<20)); finished <- err }()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("upload subprocess never became ready")
	}
	if err := h.Stop(context.Background()); err != nil {
		t.Fatal("stop did not drain subprocess", err)
	}
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("canceled upload passed")
		}
	case <-time.After(time.Second):
		t.Fatal("input pipe remained active after stop")
	}
}

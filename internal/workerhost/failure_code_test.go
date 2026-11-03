package workerhost

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func fileCode(t *testing.T, err error) string {
	t.Helper()
	var coded interface{ SandboxFailureCode() string }
	if err == nil || !errors.As(err, &coded) {
		t.Fatalf("missing category: %v", err)
	}
	if strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("host retained raw error")
	}
	return coded.SandboxFailureCode()
}

func TestFileFailureCategoryAndContextCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, code         string
		cause              error
		inspect, preserved bool
	}{
		{"inspect-deadline", "deadline", context.DeadlineExceeded, true, false},
		{"inspect-command", "command_exit", &commandFailure{code: "command_exit"}, true, false},
		{"transfer-deadline", "deadline", context.DeadlineExceeded, false, true},
		{"transfer-canceled", "canceled", context.Canceled, false, true},
		{"transfer-command", "output_limit", &commandFailure{code: "output_limit"}, false, false},
		{"transfer-unknown", "unknown", errors.New("PRIVATE-transfer"), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := opened(t)
			if tc.inspect {
				h.run = func(context.Context, io.Reader, string, ...string) ([]byte, error) { return nil, tc.cause }
			} else {
				h.stream = func(context.Context, io.Reader, io.Writer, int64, string, ...string) error { return tc.cause }
			}
			_, err := h.ReadFile(context.Background(), "owned", 8)
			if got := fileCode(t, err); got != tc.code {
				t.Fatalf("code %q want %q", got, tc.code)
			}
			if errors.Is(err, tc.cause) != tc.preserved {
				t.Fatal("legacy host cause contract changed")
			}
		})
	}
	for _, tc := range []struct{ raw, code string }{
		{"body\n404", "http_rejected"}, {"{\n200", "decode"}, {"\"../foreign\"\n200", "ownership"},
	} {
		h, _ := opened(t)
		h.stream = func(_ context.Context, _ io.Reader, out io.Writer, _ int64, _ string, _ ...string) error {
			_, err := io.WriteString(out, tc.raw)
			return err
		}
		_, err := h.UploadFile(context.Background(), nil)
		if fileCode(t, err) != tc.code {
			t.Fatal("file receipt category changed")
		}
	}
	// Stop deliberately retains its smaller command-category vocabulary.
	for _, code := range []string{"budget", "ownership", "decode", "http_rejected", "PRIVATE"} {
		if projectedFailureCode(code) != "unknown" {
			t.Fatal("file category widened Stop projection")
		}
	}
}

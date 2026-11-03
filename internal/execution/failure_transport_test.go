package execution

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type evidenceBodyError struct{ cause error }

func (r evidenceBodyError) Read([]byte) (int, error) { return 0, r.cause }
func (evidenceBodyError) Close() error               { return nil }

func TestControlFailureCategoryKeepsOldErrorContracts(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		transport  error
		body       io.ReadCloser
		status     int
		syntax     bool
	}{
		{name: "transport-deadline", code: "deadline", transport: context.DeadlineExceeded},
		{name: "transport-canceled", code: "canceled", transport: context.Canceled},
		{name: "body-deadline", code: "deadline", body: evidenceBodyError{context.DeadlineExceeded}},
		{name: "body-unknown", code: "unknown", body: evidenceBodyError{errors.New("PRIVATE-body")}},
		{name: "http", code: "http_rejected", status: 503},
		{name: "oversize", code: "output_limit", body: io.NopCloser(strings.NewReader(strings.Repeat("x", 128*1024+1)))},
		{name: "decode", code: "decode", body: io.NopCloser(strings.NewReader("{")), syntax: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGoJudge(transportFunc(func(*http.Request) (*http.Response, error) {
				if tc.transport != nil {
					return nil, tc.transport
				}
				r := response("{}")
				if tc.body != nil {
					r.Body = tc.body
				}
				if tc.status != 0 {
					r.StatusCode = tc.status
				}
				return r, nil
			}), nil)
			var target any
			err := g.request(context.Background(), "POST", "/run", nil, &target)
			if err == nil || errorFailureCode(err) != tc.code || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("control sanitization/category/context contract: %v", err)
			}
			var syntax *json.SyntaxError
			if errors.As(err, &syntax) != tc.syntax {
				t.Fatal("JSON error type compatibility changed")
			}
		})
	}
}

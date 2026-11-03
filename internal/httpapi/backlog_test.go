package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

type backlogErrorStore struct {
	sourceStore
	err error
}

func (s *backlogErrorStore) Accept(context.Context, string, string, string, submission.Request) (submission.Receipt, bool, error) {
	return submission.Receipt{}, false, s.err
}

func TestSubmissionAdmissionErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{submission.ErrBacklog, 429, "BACKLOG_LIMIT"}, {submission.ErrAdmissionPolicy, 503, "ADMISSION_UNAVAILABLE"}, {errors.New("database probe error"), 503, "ACCEPTANCE_UNKNOWN"}} {
		t.Run(tc.code, func(t *testing.T) {
			const credential = "synthetic-test-only-not-a-real-credential"
			h := httpapi.NewHandler(httpapi.Experiment{Store: &backlogErrorStore{err: tc.err}, Token: credential})
			r := httptest.NewRequest(http.MethodPost, "/api/v1/experiments/submissions", strings.NewReader(`{"material_version":"m0-sum-v1","language":"gnu++17","source":"// synthetic"}`))
			r.Header.Set("Authorization", "Bearer "+credential)
			r.Header.Set("Idempotency-Key", "backlog-error-operation")
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "database probe error") {
				t.Fatalf("unexpected response: status=%d", w.Code)
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Retry-After") != "" || w.Header().Get("Set-Cookie") != "" {
				t.Fatal("admission error changed privacy or invented retry timing")
			}
		})
	}
}

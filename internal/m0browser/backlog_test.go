package m0browser

import (
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

func TestBrowserAdmissionErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{submission.ErrBacklog, 429, "BACKLOG_LIMIT"}, {submission.ErrAdmissionPolicy, 503, "ADMISSION_UNAVAILABLE"}, {errors.New("database probe error"), 503, "ACCEPTANCE_UNKNOWN"}} {
		t.Run(tc.code, func(t *testing.T) {
			h, store, nonce := fixture(t)
			store.err = tc.err
			w := call(t, h, "POST", "/m0/api/submissions", `{"preset":"sum"}`, nonce, nil)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "database probe error") || store.writes != 1 {
				t.Fatal("browser admission contract changed")
			}
			if w.Header().Get("Retry-After") != "" {
				t.Fatal("invented retry delay")
			}
		})
	}
}

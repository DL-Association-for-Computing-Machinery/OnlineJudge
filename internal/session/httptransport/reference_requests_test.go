package httptransport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

func TestReferenceBodyRejectsAmbiguousOrUnboundedInput(t *testing.T) {
	const key = "original-reference-request"
	id := strings.Repeat("a", 32)
	valid := `{"request_key":"` + key + `","draft_id":"` + id + `"}`
	for name, tc := range map[string]struct {
		body, media string
		status      int
	}{
		"valid":              {valid, "application/json", 200},
		"wrong type":         {valid, "text/plain", 415},
		"limit":              {valid + strings.Repeat(" ", 1024), "application/json", 413},
		"trailing object":    {valid + `{}`, "application/json", 400},
		"null":               {`null`, "application/json", 400},
		"escaped duplicate":  {strings.Replace(valid, `"draft_id"`, `"request_\u006bey":"other-reference-key","draft_id"`, 1), "application/json", 400},
		"unpaired surrogate": {strings.Replace(valid, key, `\uD800`, 1), "application/json", 400},
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, referenceCollection, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.media)
			w := httptest.NewRecorder()
			gotKey, gotID, ok := referenceRequestBody(w, r)
			if w.Code != tc.status || ok != (tc.status == 200) || (ok && (gotKey != key || gotID != id)) {
				t.Fatal("wrong body admission", w.Code, ok)
			}
		})
	}
}

func TestReferenceUnknownWriteWinsOverDeadlineAndRefusal(t *testing.T) {
	w := httptest.NewRecorder()
	referenceRequestError(w, errors.Join(problemreference.ErrCommitUnknown, context.DeadlineExceeded, problemreference.ErrNotFound))
	if w.Code != 503 || !strings.Contains(w.Body.String(), "REFERENCE_WRITE_UNKNOWN") {
		t.Fatal("unknown write was reported as a definite refusal")
	}
}

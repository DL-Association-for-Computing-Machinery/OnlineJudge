package workerclient

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

func TestReferenceClaimRejectsUnknownAndMismatchedRepliesWithoutRetry(t *testing.T) {
	input := problemreference.ClaimInput{RequestID: strings.Repeat("1", 32), SourceID: "library-checker:aplusb", PlanSHA256: strings.Repeat("a", 64)}
	for _, tc := range []string{"lost", "redirect", "wrong digest", "oversized"} {
		t.Run(tc, func(t *testing.T) {
			var calls atomic.Int32
			a := acquisitionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var got problemreference.ClaimInput
				if r.URL.Path != "/api/v1/worker/reference/claim" || json.NewDecoder(r.Body).Decode(&got) != nil || got != input {
					t.Error("invalid targeted claim request")
				}
				switch tc {
				case "lost":
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = conn.Close()
				case "redirect":
					http.Redirect(w, r, "/must-not-follow", http.StatusTemporaryRedirect)
				case "wrong digest":
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(problemreference.WorkerClaim{RequestID: input.RequestID, SourceID: input.SourceID,
						PlanSHA256: strings.Repeat("b", 64), Token: strings.Repeat("c", 64), Until: time.Now().Add(time.Minute)})
				case "oversized":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(strings.Repeat(" ", 1025)))
				}
			}))
			got, err := a.ClaimReference(t.Context(), input)
			if got != (problemreference.WorkerClaim{}) || !errors.Is(err, ErrClaimUnknown) || calls.Load() != 1 {
				t.Fatal("unknown claim was accepted or retried", err, calls.Load())
			}
		})
	}
}

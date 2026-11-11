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
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

func TestReferenceRenewalRequiresExactAcknowledgement(t *testing.T) {
	claim := problemreference.WorkerClaim{RequestID: strings.Repeat("1", 32), SourceID: "library-checker:aplusb", PlanSHA256: strings.Repeat("a", 64), Token: strings.Repeat("b", 64), Until: time.Now().Add(time.Minute)}
	for _, tc := range []struct {
		kind string
		want error
	}{
		{"accepted", nil}, {"token mismatch", ErrClaimUnknown}, {"missing deadline", ErrClaimUnknown},
		{"unavailable", problemreference.ErrClaimUnavailable}, {"wrong node", submission.ErrNode},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			var calls atomic.Int32
			a := acquisitionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var input problemreference.MaterialInput
				if r.URL.Path != "/api/v1/worker/reference/renew" || r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&input) != nil || !claim.ValidFor(input.ClaimInput) || input.Token != claim.Token {
					t.Error("incorrect request binding")
				}
				reply := claim
				switch tc.kind {
				case "token mismatch":
					reply.Token = strings.Repeat("c", 64)
				case "missing deadline":
					reply.Until = time.Time{}
				case "unavailable":
					w.WriteHeader(http.StatusConflict)
					return
				case "wrong node":
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(reply); err != nil {
					t.Error(err)
				}
			}))
			if err := a.RenewReference(t.Context(), claim); !errors.Is(err, tc.want) || calls.Load() != 1 {
				t.Fatalf("wrong renewal result or retry: %v calls=%d", err, calls.Load())
			}
		})
	}
}

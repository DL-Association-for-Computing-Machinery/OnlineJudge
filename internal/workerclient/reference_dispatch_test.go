package workerclient

import (
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

func TestReferenceDispatchStopsOnAmbiguousResponse(t *testing.T) {
	input := problemreference.DispatchInput{SourceID: "library-checker:aplusb", After: strings.Repeat("1", 32)}
	for _, body := range []string{
		`{"claim":null,"next_after":"`,
		`{"next_after":""}`,
		`{"claim":null,"next_after":"` + input.After + `"}`,
		`{"claim":{"request_id":"` + strings.Repeat("2", 32) + `","source_id":"library-checker:unionfind","plan_sha256":"` + strings.Repeat("a", 64) +
			`","lease_token":"` + strings.Repeat("b", 64) + `","lease_until":"2026-11-11T00:00:00Z"},"next_after":""}`,
	} {
		var calls atomic.Int32
		a := acquisitionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/worker/reference/dispatch" {
				t.Error("wrong dispatch method or endpoint")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		got, err := a.DispatchReference(t.Context(), input)
		if got.Claim != nil || got.After != "" || !errors.Is(err, ErrClaimUnknown) || calls.Load() != 1 {
			t.Fatal("ambiguous reply did not stop dispatch", err)
		}
	}
}

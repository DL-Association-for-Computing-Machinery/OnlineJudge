package workerclient

import (
	"context"
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// ClaimReference makes one bounded reservation request. No response grants
// material access or permission to execute; unknown outcomes are not retried.
func (a *Acquirer) ClaimReference(ctx context.Context, input problemreference.ClaimInput) (problemreference.WorkerClaim, error) {
	var zero problemreference.WorkerClaim
	if a == nil || ctx == nil || !input.Valid() {
		return zero, problemreference.ErrInvalid
	}
	resp, err := a.post(ctx, "reference/claim", input)
	if err != nil {
		return zero, ErrClaimUnknown
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return zero, submission.ErrNode
	case http.StatusConflict:
		return zero, problemreference.ErrClaimUnavailable
	case http.StatusOK:
	default:
		return zero, ErrClaimUnknown
	}
	var claim problemreference.WorkerClaim
	if readJSON(resp, 1024, &claim) != nil || !claim.ValidFor(input) {
		return zero, ErrClaimUnknown
	}
	return claim, nil
}

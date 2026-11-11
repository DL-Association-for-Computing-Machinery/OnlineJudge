package workerclient

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// DispatchReference sends one bounded selection request. An unknown result
// cannot authorize a new selection or execution.
func (a *Acquirer) DispatchReference(ctx context.Context, input problemreference.DispatchInput) (problemreference.DispatchResult, error) {
	var zero problemreference.DispatchResult
	if a == nil || ctx == nil || !input.Valid() {
		return zero, problemreference.ErrInvalid
	}
	resp, err := a.post(ctx, "reference/dispatch", input)
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
	var wire struct {
		Claim json.RawMessage `json:"claim"`
		After *string         `json:"next_after"`
	}
	if readJSON(resp, 1024, &wire) != nil || wire.After == nil || ctx.Err() != nil {
		return zero, ErrClaimUnknown
	}
	result := problemreference.DispatchResult{After: *wire.After}
	if json.Unmarshal(wire.Claim, &result.Claim) != nil || !result.ValidFor(input) {
		return zero, ErrClaimUnknown
	}
	return result, nil
}

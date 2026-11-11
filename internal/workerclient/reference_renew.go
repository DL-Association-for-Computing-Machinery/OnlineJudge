package workerclient

import (
	"context"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

// RenewReference makes one three-second request and accepts only the exact
// original binding. The execution owner supplies the monotonic budget and must
// stop on any unknown acknowledgement. Use a separate client from downloads.
func (a *Acquirer) RenewReference(ctx context.Context, claim problemreference.WorkerClaim) error {
	input := problemreference.MaterialInput{ClaimInput: problemreference.ClaimInput{RequestID: claim.RequestID, SourceID: claim.SourceID, PlanSHA256: claim.PlanSHA256}, Token: claim.Token}
	if a == nil || ctx == nil || !input.Valid() || !claim.ValidFor(input.ClaimInput) {
		return problemreference.ErrInvalid
	}
	resp, err := a.post(ctx, "reference/renew", input)
	if err != nil {
		return ErrClaimUnknown
	}
	defer func() { _ = resp.Body.Close() }()
	if err := referenceMaterialStatus(resp.StatusCode); err != nil {
		return err
	}
	var renewed problemreference.WorkerClaim
	if readJSON(resp, 1024, &renewed) != nil || !renewed.ValidFor(input.ClaimInput) || renewed.Token != claim.Token || ctx.Err() != nil {
		return ErrClaimUnknown
	}
	return nil
}

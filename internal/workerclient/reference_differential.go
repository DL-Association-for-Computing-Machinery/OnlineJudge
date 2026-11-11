package workerclient

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

// FetchDifferential downloads the two reviewed solver objects once, within
// the original descriptor deadline. It grants no execution permission; a
// caller that runs programs must separately supervise the live reservation.
func (a *Acquirer) FetchDifferential(ctx context.Context, claim problemreference.WorkerClaim) (problemvalidation.ConvolutionSources, error) {
	var zero problemvalidation.ConvolutionSources
	input := problemreference.MaterialInput{ClaimInput: problemreference.ClaimInput{
		RequestID: claim.RequestID, SourceID: claim.SourceID, PlanSHA256: claim.PlanSHA256,
	}, Token: claim.Token}
	if a == nil || ctx == nil || input.SourceID != problemvalidation.ConvolutionSourceID || !claim.ValidFor(input.ClaimInput) || !input.Valid() {
		return zero, problemreference.ErrInvalid
	}
	start := time.Now()
	resp, err := a.referencePost(ctx, "reference/differential/assignment", input)
	if err != nil {
		return zero, ErrMaterials
	}
	if resp.StatusCode == http.StatusNoContent {
		if err := resp.Body.Close(); err != nil || ctx.Err() != nil {
			return zero, ErrMaterials
		}
		return zero, problemreference.ErrDifferentialNotPrepared
	}
	var descriptor problemreference.DifferentialDescriptor
	readErr := readJSON(resp, 2*problemreference.MaxDifferentialPlanBytes, &descriptor)
	closeErr := resp.Body.Close()
	if err := referenceMaterialStatus(resp.StatusCode); err != nil {
		return zero, err
	}
	if readErr != nil || closeErr != nil || descriptor.RemainingMS <= 0 || descriptor.RemainingMS > 30000 {
		return zero, ErrMaterials
	}
	deadline := start.Add(time.Duration(descriptor.RemainingMS) * time.Millisecond)
	plan, err := descriptor.CurrentPlan(claim.PlanSHA256)
	if err != nil || !time.Now().Before(deadline) {
		return zero, ErrMaterials
	}
	download, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	sources, err := plan.FreezePrograms(download, claim.PlanSHA256, func(ctx context.Context, o materialstore.Object) (io.ReadCloser, error) {
		return a.fetchReferenceObject(ctx, "reference/differential/material", struct {
			problemreference.MaterialInput
			Object materialstore.Object `json:"object"`
			Digest string               `json:"differential_sha256"`
		}{input, o, descriptor.SHA256}, o)
	})
	if err != nil || download.Err() != nil || !time.Now().Before(deadline) {
		return zero, ErrMaterials
	}
	return sources, nil
}

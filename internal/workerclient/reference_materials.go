package workerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// VerifiedReference owns a complete anonymous snapshot bound to its context.
// The snapshot does not grant execution and cannot be rebound.
type VerifiedReference struct {
	*problem.ReferenceSnapshot
	cancel context.CancelFunc
}

// Close releases private disk bytes and the associated deadline context.
func (v *VerifiedReference) Close() error { v.cancel(); return v.ReferenceSnapshot.Close() }

// FetchReference downloads and hashes the complete expected material set once,
// without caching, retries, renewal or execution. The caller must Close it.
func (a *Acquirer) FetchReference(ctx context.Context, claim problemreference.WorkerClaim) (*VerifiedReference, error) {
	return a.fetchReference(ctx, claim, false)
}

// FetchReferenceForExecution binds the snapshot to a single lease-supervised
// context while still enforcing the descriptor's original download deadline.
// It is only for a caller that holds the slot and cancels ctx on authority loss.
func (a *Acquirer) FetchReferenceForExecution(ctx context.Context, claim problemreference.WorkerClaim) (*VerifiedReference, error) {
	return a.fetchReference(ctx, claim, true)
}

func (a *Acquirer) fetchReference(ctx context.Context, claim problemreference.WorkerClaim, supervised bool) (*VerifiedReference, error) {
	input := problemreference.MaterialInput{ClaimInput: problemreference.ClaimInput{RequestID: claim.RequestID, SourceID: claim.SourceID, PlanSHA256: claim.PlanSHA256}, Token: claim.Token}
	if a == nil || ctx == nil || !claim.ValidFor(input.ClaimInput) || !input.Valid() {
		return nil, problemreference.ErrInvalid
	}
	start := time.Now()
	resp, err := a.referencePost(ctx, "reference/assignment", input)
	if err != nil {
		return nil, ErrMaterials
	}
	var d problemreference.MaterialDescriptor
	readErr := readJSON(resp, 2*problemreference.MaxExpectedPlanBytes, &d)
	_ = resp.Body.Close()
	if err := referenceMaterialStatus(resp.StatusCode); err != nil {
		return nil, err
	}
	if readErr != nil || len(d.Encoding) > problemreference.MaxExpectedPlanBytes || checksum(d.Encoding) != input.PlanSHA256 || d.RemainingMS <= 0 || d.RemainingMS > 30000 {
		return nil, ErrMaterials
	}
	var plan problem.ReferencePlan
	if json.Unmarshal(d.Encoding, &plan) != nil {
		return nil, ErrMaterials
	}
	raw, err := json.Marshal(plan)
	if err != nil || !bytes.Equal(raw, d.Encoding) || plan.SourceID != input.SourceID || plan.SourceLockSHA256 != problemvalidation.SourceLockSHA256 || plan.ValidateMaterials() != nil {
		return nil, ErrMaterials
	}
	deadline := start.Add(time.Duration(d.RemainingMS) * time.Millisecond)
	if !time.Now().Before(deadline) {
		return nil, ErrMaterials
	}
	open := func(ctx context.Context, o materialstore.Object) (io.ReadCloser, error) {
		return a.referenceObject(ctx, input, o)
	}
	var snapshot *problem.ReferenceSnapshot
	var lifetime context.Context
	var cancel context.CancelFunc
	if supervised {
		lifetime, cancel = context.WithCancel(ctx)
		snapshot, err = plan.FreezeMaterialsWithDownloadDeadline(lifetime, deadline, open)
	} else {
		lifetime, cancel = context.WithDeadline(ctx, deadline)
		snapshot, err = plan.FreezeMaterials(lifetime, open)
	}
	if err != nil {
		cancel()
		return nil, ErrMaterials
	}
	if lifetime.Err() != nil || !time.Now().Before(deadline) {
		_ = snapshot.Close()
		cancel()
		return nil, ErrMaterials
	}
	return &VerifiedReference{snapshot, cancel}, nil
}

func (a *Acquirer) referencePost(ctx context.Context, action string, value any) (*http.Response, error) {
	// Reference objects can be large; M0's three-second whole-request budget
	// stays unchanged. This copy remains bounded by the live reservation.
	copyTransport := *a.transport
	client := *a.transport.client
	client.Timeout = 30 * time.Second
	copyTransport.client = &client
	return (&Acquirer{transport: &copyTransport}).post(ctx, action, value)
}

func (a *Acquirer) referenceObject(ctx context.Context, input problemreference.MaterialInput, o materialstore.Object) (io.ReadCloser, error) {
	return a.fetchReferenceObject(ctx, "reference/material", struct {
		problemreference.MaterialInput
		Object materialstore.Object `json:"object"`
	}{input, o}, o)
}

func (a *Acquirer) fetchReferenceObject(ctx context.Context, path string, input any, o materialstore.Object) (io.ReadCloser, error) {
	resp, err := a.referencePost(ctx, path, input)
	if err != nil {
		return nil, ErrMaterials
	}
	media, _, mediaErr := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err := referenceMaterialStatus(resp.StatusCode); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	if mediaErr != nil || media != "application/octet-stream" || resp.ContentLength != o.Size || resp.Header.Get("X-Content-SHA256") != o.SHA256 {
		_ = resp.Body.Close()
		return nil, ErrMaterials
	}
	return resp.Body, nil
}

func referenceMaterialStatus(status int) error {
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return submission.ErrNode
	case http.StatusConflict:
		return problemreference.ErrClaimUnavailable
	default:
		return ErrMaterials
	}
}

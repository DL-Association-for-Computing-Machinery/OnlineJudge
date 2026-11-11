package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/jackc/pgx/v5"
)

// AuthorizeReferenceMaterials is read-only and never extends the reservation.
func (c *ReferenceClaimer) AuthorizeReferenceMaterials(ctx context.Context, node submission.NodeIdentity, input problemreference.MaterialInput) (problemreference.MaterialAuthorization, error) {
	var zero problemreference.MaterialAuthorization
	if c == nil || ctx == nil || !input.Valid() {
		return zero, problemreference.ErrInvalid
	}
	if node.ID != c.nodeID || node.CredentialVersion < 1 {
		return zero, submission.ErrNode
	}
	profile, _, err := problemvalidation.ResolveSource(input.SourceID)
	if err != nil {
		return zero, problemreference.ErrClaimUnavailable
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		return zero, storageError{err}
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := c.store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, storageError{err}
	}
	defer rollback(tx)
	if err := c.lockNode(ctx, tx, node); err != nil {
		return zero, err
	}
	q := referencedb.New(tx)
	r, err := c.lockClaimRequest(ctx, tx, q, input.ClaimInput, raw)
	if err != nil {
		return zero, err
	}
	p, err := q.ReferenceExpectedPlanForShare(ctx, r.ID)
	if err != nil {
		return zero, referenceClaimError(err)
	}
	plan := problemreference.ExpectedPlan{Encoding: p.PlanEncoding, SHA256: p.PlanSha256}
	var parsed problem.ReferencePlan
	if plan.SHA256 != input.PlanSHA256 || !plan.ValidFor(r.Specification) || json.Unmarshal(plan.Encoding, &parsed) != nil || parsed.ValidateMaterials() != nil {
		return zero, problemreference.ErrClaimUnavailable
	}
	claim, err := q.ReferenceWorkerClaimForShare(ctx)
	if err != nil {
		return zero, referenceClaimError(err)
	}
	if claim.RequestID != r.ID || claim.NodeID != node.ID || claim.CredentialVersion != node.CredentialVersion || claim.LeaseToken != input.Token || claim.PlanSha256 != input.PlanSHA256 {
		return zero, problemreference.ErrClaimUnavailable
	}
	ms, err := q.ReferenceWorkerClaimRemaining(ctx)
	if err != nil {
		return zero, referenceClaimError(err)
	}
	if ms <= 0 || ms > 30000 {
		return zero, problemreference.ErrClaimUnavailable
	}
	deadline := start.Add(time.Duration(ms) * time.Millisecond)
	if !time.Now().Before(deadline) {
		return zero, problemreference.ErrClaimUnavailable
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return zero, err
	}
	if ctx.Err() != nil || !time.Now().Before(deadline) {
		return zero, problemreference.ErrClaimUnavailable
	}
	return problemreference.MaterialAuthorization{Plan: plan, Deadline: deadline}, nil
}

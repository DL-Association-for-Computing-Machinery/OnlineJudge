package postgres

import (
	"context"
	"encoding/json"
	"errors"
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
	return c.authorizeReferenceMaterials(ctx, node, input, false)
}

// AuthorizeDifferentialMaterials verifies the additional plan under the same
// live formal reservation. It never extends the reservation.
func (c *ReferenceClaimer) AuthorizeDifferentialMaterials(ctx context.Context, node submission.NodeIdentity, input problemreference.MaterialInput) (problemreference.MaterialAuthorization, error) {
	if input.SourceID != problemvalidation.ConvolutionSourceID {
		return problemreference.MaterialAuthorization{}, problemreference.ErrClaimUnavailable
	}
	return c.authorizeReferenceMaterials(ctx, node, input, true)
}

func (c *ReferenceClaimer) authorizeReferenceMaterials(ctx context.Context, node submission.NodeIdentity, input problemreference.MaterialInput, differential bool) (problemreference.MaterialAuthorization, error) {
	plan, deadline, _, err := c.liveReferenceClaim(ctx, node, input, false, differential)
	if err != nil {
		return problemreference.MaterialAuthorization{}, err
	}
	return problemreference.MaterialAuthorization{Plan: plan, Deadline: deadline}, nil
}

// RenewReference rechecks live authority and only extends the exact live token.
// It never marks execution stopped or rebinds an expired reservation.
func (c *ReferenceClaimer) RenewReference(ctx context.Context, node submission.NodeIdentity, input problemreference.MaterialInput) (problemreference.WorkerClaim, error) {
	_, _, until, err := c.liveReferenceClaim(ctx, node, input, true, false)
	if err != nil {
		return problemreference.WorkerClaim{}, err
	}
	return problemreference.WorkerClaim{RequestID: input.RequestID, SourceID: input.SourceID,
		PlanSHA256: input.PlanSHA256, Token: input.Token, Until: until}, nil
}

func (c *ReferenceClaimer) liveReferenceClaim(ctx context.Context, node submission.NodeIdentity, input problemreference.MaterialInput, renew, differential bool) (problemreference.ExpectedPlan, time.Time, time.Time, error) {
	var zero problemreference.ExpectedPlan
	var noTime time.Time
	if c == nil || ctx == nil || !input.Valid() {
		return zero, noTime, noTime, problemreference.ErrInvalid
	}
	if node.ID != c.nodeID || node.CredentialVersion < 1 {
		return zero, noTime, noTime, submission.ErrNode
	}
	profile, err := c.resolveProfile(input.SourceID)
	if err != nil {
		return zero, noTime, noTime, problemreference.ErrClaimUnavailable
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		return zero, noTime, noTime, storageError{err}
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := c.store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, noTime, noTime, storageError{err}
	}
	defer rollback(tx)
	if err := c.lockNode(ctx, tx, node); err != nil {
		return zero, noTime, noTime, err
	}
	q := referencedb.New(tx)
	r, err := c.lockClaimRequest(ctx, tx, q, input.ClaimInput, raw)
	if err != nil {
		return zero, noTime, noTime, err
	}
	p, err := q.ReferenceExpectedPlanForShare(ctx, r.ID)
	if err != nil {
		return zero, noTime, noTime, referenceClaimError(err)
	}
	plan := problemreference.ExpectedPlan{Encoding: p.PlanEncoding, SHA256: p.PlanSha256}
	var parsed problem.ReferencePlan
	if plan.SHA256 != input.PlanSHA256 || !plan.ValidFor(r.Specification) || json.Unmarshal(plan.Encoding, &parsed) != nil || parsed.ValidateMaterials() != nil {
		return zero, noTime, noTime, problemreference.ErrClaimUnavailable
	}
	if differential {
		if err := loadCurrentComparison(ctx, q, r, &plan); err != nil {
			return zero, noTime, noTime, err
		}
	}
	var claim referencedb.ProblemReferenceWorkerClaim
	if renew {
		claim, err = q.ReferenceWorkerClaimForUpdate(ctx)
	} else {
		claim, err = q.ReferenceWorkerClaimForShare(ctx)
	}
	if err != nil {
		return zero, noTime, noTime, referenceClaimError(err)
	}
	if claim.RequestID != r.ID || claim.NodeID != node.ID || claim.CredentialVersion != node.CredentialVersion || claim.LeaseToken != input.Token || claim.PlanSha256 != input.PlanSHA256 {
		return zero, noTime, noTime, problemreference.ErrClaimUnavailable
	}
	if renew {
		until, err := q.RenewReferenceWorkerClaim(ctx, referencedb.RenewReferenceWorkerClaimParams{
			RequestID: r.ID, NodeID: node.ID, CredentialVersion: node.CredentialVersion, LeaseToken: input.Token, PlanSha256: input.PlanSHA256,
		})
		if err != nil {
			return zero, noTime, noTime, referenceClaimError(err)
		}
		if !until.Valid || until.InfinityModifier != 0 {
			return zero, noTime, noTime, storageError{problemreference.ErrInvalid}
		}
		if err := commitTx(ctx, tx, true); err != nil {
			return zero, noTime, noTime, err
		}
		if err := ctx.Err(); err != nil {
			return zero, noTime, noTime, errors.Join(problemreference.ErrCommitUnknown, err)
		}
		return plan, noTime, until.Time, nil
	}
	ms, err := q.ReferenceWorkerClaimRemaining(ctx)
	if err != nil {
		return zero, noTime, noTime, referenceClaimError(err)
	}
	if ms <= 0 || ms > 30000 {
		return zero, noTime, noTime, problemreference.ErrClaimUnavailable
	}
	deadline := start.Add(time.Duration(ms) * time.Millisecond)
	if !time.Now().Before(deadline) {
		return zero, noTime, noTime, problemreference.ErrClaimUnavailable
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return zero, noTime, noTime, err
	}
	if ctx.Err() != nil || !time.Now().Before(deadline) {
		return zero, noTime, noTime, problemreference.ErrClaimUnavailable
	}
	return plan, deadline, noTime, nil
}

// The caller must already hold the formal plan's row lock. Only both NULL
// fields mean historical absence; corrupted or retired data never downgrades.
func loadCurrentComparison(ctx context.Context, q *referencedb.Queries, r problemreference.Request, plan *problemreference.ExpectedPlan) error {
	row, err := q.ReferenceConvolutionExpectedPlan(ctx, r.ID)
	if err != nil {
		return referenceClaimError(err)
	}
	if !row.DifferentialSha256.Valid && row.DifferentialEncoding == nil {
		return nil
	}
	plan.Differential = &problemreference.DifferentialExpectedPlan{Encoding: row.DifferentialEncoding, SHA256: row.DifferentialSha256.String}
	if !plan.ValidForRequest(r) {
		return problemreference.ErrClaimUnavailable
	}
	_, err = plan.Differential.CurrentPlan(plan.SHA256)
	return err
}

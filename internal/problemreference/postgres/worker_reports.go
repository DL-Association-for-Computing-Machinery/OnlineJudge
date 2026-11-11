package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/jackc/pgx/v5"
)

// ReportReference records one immutable result and releases the exact live
// reservation in the same transaction. Exact replays cannot mutate either.
func (c *ReferenceClaimer) ReportReference(ctx context.Context, node submission.NodeIdentity, input problemreference.WorkerReportInput) error {
	if c == nil || ctx == nil {
		return problemreference.ErrInvalid
	}
	if node.ID != c.nodeID || node.CredentialVersion < 1 {
		return submission.ErrNode
	}
	evidence, err := input.DecodeEvidence()
	if err != nil {
		return err
	}
	var rawProfile []byte
	if profile, _, err := problemvalidation.ResolveSource(input.SourceID); err == nil {
		rawProfile, err = json.Marshal(profile)
		if err != nil {
			return storageError{err}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := c.store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return storageError{err}
	}
	defer rollback(tx)
	if err := c.lockNode(ctx, tx, node); err != nil {
		return err
	}
	q := referencedb.New(tx)
	request, err := c.lockReferenceRequest(ctx, tx, q, input.ClaimInput, rawProfile, true)
	if err != nil {
		return err
	}
	tokenDigest := sha256.Sum256([]byte(input.Token))
	tokenSHA := hex.EncodeToString(tokenDigest[:])
	if request.State == problemreference.Reported {
		saved, err := q.ReferenceWorkerReport(ctx, request.ID)
		if err != nil {
			return storageError{err}
		}
		if saved.NodeID != node.ID || saved.CredentialVersion != node.CredentialVersion || saved.TokenSha256 != tokenSHA ||
			saved.PlanSha256 != input.PlanSHA256 || !bytes.Equal(saved.Evidence, input.Evidence) {
			return problemreference.ErrClaimUnavailable
		}
		return commitTx(ctx, tx, false)
	}
	row, err := q.ReferenceExpectedPlanForShare(ctx, request.ID)
	if err != nil {
		return referenceClaimError(err)
	}
	plan := problemreference.ExpectedPlan{Encoding: row.PlanEncoding, SHA256: row.PlanSha256}
	if plan.SHA256 != input.PlanSHA256 || !evidence.ValidFor(request.Specification, plan) {
		return problemreference.ErrClaimUnavailable
	}
	if evidence.Verdict == "AC" && request.Specification.Identity.SourceID == problemvalidation.ConvolutionSourceID {
		if err := loadCurrentComparison(ctx, q, request, &plan); err != nil {
			return err
		}
	}
	if !evidence.ValidComparison(plan) {
		return problemreference.ErrClaimUnavailable
	}
	if _, err := q.ReferenceWorkerClaimForUpdate(ctx); err != nil {
		return referenceClaimError(err)
	}
	if _, err := q.ConsumeLiveReferenceClaim(ctx, referencedb.ConsumeLiveReferenceClaimParams{
		RequestID: request.ID, NodeID: node.ID, CredentialVersion: node.CredentialVersion, LeaseToken: input.Token, PlanSha256: input.PlanSHA256,
	}); err != nil {
		return referenceClaimError(err)
	}
	if err := q.InsertReferenceWorkerReport(ctx, referencedb.InsertReferenceWorkerReportParams{
		RequestID: request.ID, NodeID: node.ID, CredentialVersion: node.CredentialVersion, TokenSha256: tokenSHA, PlanSha256: input.PlanSHA256, Evidence: input.Evidence,
	}); err != nil {
		return storageError{err}
	}
	if n, err := q.MarkReferenceReported(ctx, request.ID); err != nil || n != 1 {
		return storageError{errors.Join(problemreference.ErrInvalid, err)}
	}
	if err := commitTx(ctx, tx, true); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(problemreference.ErrCommitUnknown, err)
	}
	return nil
}

package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/jackc/pgx/v5"
)

// ReviewDraftReader loads metadata for the immutable owner/draft selected by
// the already-authorized item. It must not do I/O outside this database tx.
type ReviewDraftReader func(context.Context, pgx.Tx, string, string) (problem.Draft, error)

// ReviewMaterialStore reuses the summary's exact authorization and reads the
// private metadata needed to prepare only the bounded review selection.
type ReviewMaterialStore struct {
	reviews *ReviewStore
	drafts  ReviewDraftReader
}

// NewReviewMaterialStore binds a same-transaction draft metadata adapter.
func NewReviewMaterialStore(reviews *ReviewStore, drafts ReviewDraftReader) (*ReviewMaterialStore, error) {
	if reviews == nil || drafts == nil {
		return nil, problemreference.ErrInvalid
	}
	return &ReviewMaterialStore{reviews: reviews, drafts: drafts}, nil
}

// ReviewMaterialSnapshot reconstructs the authorized selection without any
// object or session I/O. Callers must recheck it after material reads.
func (s *ReviewMaterialStore) ReviewMaterialSnapshot(ctx context.Context, actor identity.ProblemActor, id string) (problemreference.ReviewMaterialSnapshot, error) {
	var zero problemreference.ReviewMaterialSnapshot
	if s == nil || s.drafts == nil || !s.reviews.validRead(ctx, actor, id) {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.reviews.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, storageError{err}
	}
	defer rollback(tx)
	item, err := s.reviews.lockAssignedReview(ctx, tx, actor, id)
	if err != nil {
		return zero, err
	}
	draft, err := s.drafts(ctx, tx, item.evidence.DraftID, item.owner)
	if err != nil {
		return zero, storageError{err}
	}
	frozen, err := loadHistoricalReviewEvidence(ctx, referencedb.New(tx), item)
	if err != nil {
		return zero, storageError{err}
	}
	var plan problem.ReferencePlan
	if err := json.Unmarshal(frozen.expected.Encoding, &plan); err != nil {
		return zero, storageError{err}
	}
	manifest, err := draft.PrepareReviewMaterials(item.evidence.DraftID, item.owner, frozen.bound, plan)
	if err != nil {
		return zero, storageError{err}
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return problemreference.ReviewMaterialSnapshot{Evidence: item.evidence, Owner: item.owner,
		RoleRevision: item.roleRevision, Manifest: manifest}, nil
}

type historicalReviewEvidence struct {
	bound      problem.ImportIdentity
	expected   problemreference.ExpectedPlan
	comparison *problemreference.AuthorComparisonResult
}

func loadHistoricalReviewEvidence(ctx context.Context, q *referencedb.Queries, item assignedReview) (historicalReviewEvidence, error) {
	var zero historicalReviewEvidence
	e := item.evidence
	row, err := q.ReportedReferenceForReview(ctx, referencedb.ReportedReferenceForReviewParams{
		ID: e.ReferenceRequestID, Owner: item.owner, Draft: e.DraftID})
	if err != nil {
		return zero, err
	}
	r, err := readRequest(row)
	if err != nil {
		return zero, err
	}
	bound := problem.ImportIdentity{SourceID: e.SourceID, Version: e.ContentVersion,
		ImportRevision: e.ImportRevision, SourceLockSHA256: e.SourceLockSHA256}
	if r.Specification.Identity != bound || r.Specification.ProfileSHA256 != e.ProfileSHA256 {
		return zero, problemreference.ErrInvalid
	}
	expected, err := loadExpectedPlan(ctx, q, r)
	if err != nil {
		return zero, err
	}
	report, err := q.ReferenceWorkerReport(ctx, r.ID)
	if err != nil {
		return zero, err
	}
	projected, err := problemreference.ProjectAuthorReport(r, expected, report.PlanSha256, report.EvidenceSha256, report.Evidence)
	if err != nil {
		return zero, err
	}
	var diffPlan, diffOutcome string
	if expected.Differential != nil {
		diffPlan = expected.Differential.SHA256
	}
	if projected.Comparison != nil {
		diffOutcome = projected.Comparison.Status
	}
	if expected.SHA256 != e.FormalPlanSHA256 || report.EvidenceSha256 != e.EvidenceSHA256 ||
		projected.Formal.Verdict != e.FormalVerdict || diffPlan != e.ComparisonPlan || diffOutcome != e.ComparisonOutcome {
		return zero, problemreference.ErrInvalid
	}
	return historicalReviewEvidence{bound: bound, expected: expected, comparison: projected.Comparison}, nil
}

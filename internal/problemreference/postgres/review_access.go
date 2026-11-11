package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReviewerAuthority locks identity-owned facts in the caller's transaction.
type ReviewerAuthority interface {
	LockReviewer(context.Context, pgx.Tx, identity.ProblemActor) (int64, error)
	LockReviewerForAssignment(context.Context, pgx.Tx, string) (int64, error)
}

// ReviewStore owns assignment changes and the assigned review projection.
type ReviewStore struct {
	pool *pgxpool.Pool
	auth ReviewerAuthority
}

// NewReviewStore binds review authorization to a caller-owned pool.
func NewReviewStore(pool *pgxpool.Pool, auth ReviewerAuthority) (*ReviewStore, error) {
	if pool == nil || auth == nil {
		return nil, problemreference.ErrInvalid
	}
	return &ReviewStore{pool: pool, auth: auth}, nil
}

// Assignment observes the latest maintenance revision for one review item.
func (s *ReviewStore) Assignment(ctx context.Context, id string) (problemreference.ReviewAssignment, error) {
	if s == nil || s.pool == nil || ctx == nil || !identity.ValidID(id) {
		return problemreference.ReviewAssignment{}, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return readAssignment(ctx, referencedb.New(s.pool), id)
}

// SetAssignment appends one trusted maintenance change. An assignment locks
// the new reviewer before the item; withdrawal only locks the item.
func (s *ReviewStore) SetAssignment(ctx context.Context, c problemreference.ReviewAssignmentChange) (problemreference.ReviewAssignment, error) {
	var zero problemreference.ReviewAssignment
	if s == nil || s.pool == nil || s.auth == nil || ctx == nil || !c.Valid() {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, storageError{err}
	}
	defer rollback(tx)
	var reviewerRev int64
	if c.ReviewerSubjectID != "" {
		reviewerRev, err = s.auth.LockReviewerForAssignment(ctx, tx, c.ReviewerSubjectID)
		if err != nil {
			return zero, err
		}
		if reviewerRev != c.ExpectedReviewerRevision {
			return zero, problemreference.ErrConflict
		}
	}
	q := referencedb.New(tx)
	row, err := q.ReviewSubmissionForUpdate(ctx, c.ReviewID)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	if row.OwnerSubjectID == c.ReviewerSubjectID {
		return zero, identity.ErrReviewerRequired
	}
	// A separate statement sees a change committed while the item lock waited.
	previous, err := readAssignment(ctx, q, c.ReviewID)
	if err != nil && !errors.Is(err, problemreference.ErrNotFound) {
		return zero, err
	}
	if previous.Revision != c.ExpectedRevision || (c.ReviewerSubjectID == "" && previous.ReviewerSubjectID == "") {
		return zero, problemreference.ErrConflict
	}
	next := problemreference.ReviewAssignment{ReviewID: c.ReviewID, Revision: previous.Revision + 1,
		ReviewerSubjectID: c.ReviewerSubjectID, ReviewerRevision: reviewerRev}
	assigned := next.ReviewerSubjectID != ""
	if err := q.AppendReviewAssignment(ctx, referencedb.AppendReviewAssignmentParams{
		ReviewID: next.ReviewID, Revision: next.Revision,
		ReviewerSubjectID:   pgtype.Text{String: next.ReviewerSubjectID, Valid: assigned},
		ReviewerRevision:    pgtype.Int8{Int64: next.ReviewerRevision, Valid: assigned},
		MaintenanceOperator: c.Operator, Reason: c.Reason,
	}); err != nil {
		return zero, storageError{err}
	}
	if err := commitTx(ctx, tx, true); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, errors.Join(problemreference.ErrCommitUnknown, err)
	}
	return next, nil
}

// ReviewEvidence locks current reviewer authority and the item. Missing,
// foreign, reassigned, and withdrawn items are indistinguishable.
func (s *ReviewStore) ReviewEvidence(ctx context.Context, actor identity.ProblemActor, id string) (problemreference.ReviewEvidence, error) {
	var zero problemreference.ReviewEvidence
	if !s.validRead(ctx, actor, id) {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, storageError{err}
	}
	defer rollback(tx)
	item, err := s.lockAssignedReview(ctx, tx, actor, id)
	if err != nil {
		return zero, err
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return item.evidence, nil
}

// ReviewComparison binds the accepted report to the review item under the
// same role and assignment locks used for the evidence summary.
func (s *ReviewStore) ReviewComparison(ctx context.Context, actor identity.ProblemActor, id string) (problemreference.ReviewComparison, error) {
	var zero problemreference.ReviewComparison
	if !s.validRead(ctx, actor, id) {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, storageError{err}
	}
	defer rollback(tx)
	item, err := s.lockAssignedReview(ctx, tx, actor, id)
	if err != nil {
		return zero, err
	}
	frozen, err := loadHistoricalReviewEvidence(ctx, referencedb.New(tx), item.owner, item.evidence)
	if err != nil {
		return zero, storageError{err}
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return problemreference.ReviewComparison{Evidence: item.evidence, Comparison: frozen.comparison}, nil
}

func (s *ReviewStore) validRead(ctx context.Context, actor identity.ProblemActor, id string) bool {
	return s != nil && s.pool != nil && s.auth != nil && ctx != nil && actor.Valid() && identity.ValidID(id)
}

type assignedReview struct {
	owner        string
	roleRevision int64
	evidence     problemreference.ReviewEvidence
}

func (s *ReviewStore) lockAssignedReview(ctx context.Context, tx pgx.Tx, actor identity.ProblemActor, id string) (assignedReview, error) {
	var zero assignedReview
	roleRev, err := s.auth.LockReviewer(ctx, tx, actor)
	if err != nil {
		return zero, err
	}
	q := referencedb.New(tx)
	row, err := q.ReviewSubmissionForShare(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	if row.OwnerSubjectID == actor.SubjectID {
		return zero, problemreference.ErrNotFound
	}
	if !identity.ValidID(row.OwnerSubjectID) {
		return zero, storageError{problemreference.ErrInvalid}
	}
	a, err := readAssignment(ctx, q, id)
	if err != nil {
		return zero, err
	}
	if roleRev < 1 || a.ReviewerSubjectID != actor.SubjectID || a.ReviewerRevision != roleRev {
		return zero, problemreference.ErrNotFound
	}
	e := reviewEvidence(row)
	e.Reviewer, e.AssignmentRevision = actor.SubjectID, a.Revision
	if e.ReviewID != id || !e.Valid() {
		return zero, storageError{problemreference.ErrInvalid}
	}
	return assignedReview{owner: row.OwnerSubjectID, roleRevision: roleRev, evidence: e}, nil
}

func reviewEvidence(row referencedb.ProblemReviewSubmission) problemreference.ReviewEvidence {
	return problemreference.ReviewEvidence{ReviewID: row.ID, DraftID: row.DraftID, SourceID: row.SourceID, ContentVersion: row.ContentVersion,
		ImportRevision: row.ImportRevision, SourceLockSHA256: row.SourceLockSha256,
		ReferenceRequestID: row.ReferenceRequestID, FormalPlanSHA256: row.FormalPlanSha256,
		ProfileSHA256: row.ProfileSha256, EvidenceSHA256: row.EvidenceSha256, FormalVerdict: row.FormalVerdict,
		ComparisonPlan: row.DifferentialPlanSha256, ComparisonOutcome: row.DifferentialOutcome}
}

func readAssignment(ctx context.Context, q *referencedb.Queries, id string) (problemreference.ReviewAssignment, error) {
	var zero problemreference.ReviewAssignment
	row, err := q.LatestReviewAssignment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	if row.ReviewID != id || row.Revision < 1 || row.ReviewerSubjectID.Valid != row.ReviewerRevision.Valid ||
		(row.ReviewerSubjectID.Valid && (!identity.ValidID(row.ReviewerSubjectID.String) || row.ReviewerRevision.Int64 < 1)) {
		return zero, storageError{problemreference.ErrInvalid}
	}
	return problemreference.ReviewAssignment{ReviewID: id, Revision: row.Revision,
		ReviewerSubjectID: row.ReviewerSubjectID.String, ReviewerRevision: row.ReviewerRevision.Int64}, nil
}

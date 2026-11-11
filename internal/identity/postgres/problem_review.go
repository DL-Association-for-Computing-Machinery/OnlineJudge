package postgres

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
	"github.com/jackc/pgx/v5"
)

// LockReviewer rechecks the current session generation and latest review
// grant. The account lock serializes all changes to that grant.
func (s *ProblemAccess) LockReviewer(ctx context.Context, tx pgx.Tx, actor identity.ProblemActor) (int64, error) {
	if s == nil || ctx == nil || tx == nil || !actor.Valid() {
		return 0, identity.ErrInvalid
	}
	q := identitydb.New(tx)
	if err := lockRecovery(ctx, q, actor.RecoveryID); err != nil {
		return 0, err
	}
	account, err := q.GetStateForShare(ctx, actor.SubjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, identity.ErrConflict
	}
	if err != nil {
		return 0, storageError{err}
	}
	if account.ID != actor.SubjectID || !account.Enabled || account.AuthGeneration != actor.AuthGeneration {
		return 0, identity.ErrConflict
	}
	return reviewerRevision(ctx, q, actor.SubjectID)
}

// LockReviewerForAssignment freezes the current grant of a maintenance-selected
// subject. This does not authenticate that subject and grants no evidence access.
func (s *ProblemAccess) LockReviewerForAssignment(ctx context.Context, tx pgx.Tx, subject string) (int64, error) {
	if s == nil || ctx == nil || tx == nil || !identity.ValidID(subject) {
		return 0, identity.ErrInvalid
	}
	q := identitydb.New(tx)
	account, err := q.GetStateForShare(ctx, subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, identity.ErrReviewerRequired
	}
	if err != nil {
		return 0, storageError{err}
	}
	if account.ID != subject || !account.Enabled {
		return 0, identity.ErrReviewerRequired
	}
	return reviewerRevision(ctx, q, subject)
}

func reviewerRevision(ctx context.Context, q *identitydb.Queries, subject string) (int64, error) {
	row, err := readReviewer(ctx, q, subject)
	if errors.Is(err, identity.ErrNotFound) || (err == nil && !row.Enabled) {
		return 0, identity.ErrReviewerRequired
	}
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return row.Revision, nil
}

// ReviewerRole observes the latest maintenance revision without granting access.
func (s *ProblemAccess) ReviewerRole(ctx context.Context, subject string) (identity.ProblemReviewerRole, error) {
	if s == nil || s.pool == nil || ctx == nil || !identity.ValidID(subject) {
		return identity.ProblemReviewerRole{}, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return readReviewer(ctx, identitydb.New(s.pool), subject)
}

func readReviewer(ctx context.Context, q *identitydb.Queries, subject string) (identity.ProblemReviewerRole, error) {
	row, err := q.LatestProblemReviewer(ctx, subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ProblemReviewerRole{}, identity.ErrNotFound
	}
	if err != nil {
		return identity.ProblemReviewerRole{}, storageError{err}
	}
	if row.SubjectID != subject || row.Revision < 1 {
		return identity.ProblemReviewerRole{}, storageError{identity.ErrInvalid}
	}
	return identity.ProblemReviewerRole{SubjectID: row.SubjectID, Enabled: row.Enabled, Revision: row.Revision}, nil
}

// SetReviewerRole appends one audited trusted-maintenance CAS change. If the
// caller loses the result, it must inspect the revision before taking action.
func (s *ProblemAccess) SetReviewerRole(ctx context.Context, c identity.ProblemRoleChange) (identity.ProblemReviewerRole, error) {
	var zero identity.ProblemReviewerRole
	if s == nil || s.pool == nil || ctx == nil || !c.Valid() || c.ExpectedRevision == math.MaxInt64 {
		return zero, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, storageError{err}
	}
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(clean)
	}()
	q := identitydb.New(tx)
	account, err := q.AccountForRoleChange(ctx, c.SubjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, identity.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	if c.Enabled && !account.Enabled {
		return zero, identity.ErrReviewerRequired
	}
	previous, err := readReviewer(ctx, q, c.SubjectID)
	if err != nil && !errors.Is(err, identity.ErrNotFound) {
		return zero, err
	}
	if previous.Revision != c.ExpectedRevision {
		return zero, identity.ErrConflict
	}
	next := identity.ProblemReviewerRole{SubjectID: c.SubjectID, Enabled: c.Enabled, Revision: previous.Revision + 1}
	if err := q.AppendProblemReviewer(ctx, identitydb.AppendProblemReviewerParams{SubjectID: next.SubjectID,
		Revision: next.Revision, Enabled: next.Enabled, MaintenanceOperator: c.Operator, Reason: c.Reason}); err != nil {
		return zero, storageError{err}
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, storageError{err}
	}
	if err := ctx.Err(); err != nil {
		return zero, storageError{err}
	}
	return next, nil
}

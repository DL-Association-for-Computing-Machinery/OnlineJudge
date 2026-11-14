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

// LockPublisher checks the caller's current account and exact grant in a
// fresh caller-owned publication transaction. It locks both expected accounts
// before the review gate locks the assignment and material. On error the
// caller must roll back. This does not verify the reviewer's grant or receipt.
func (s *ProblemAccess) LockPublisher(ctx context.Context, tx pgx.Tx, actor identity.ProblemActor, expectedRevision int64, reviewer string) error {
	if s == nil || ctx == nil || tx == nil || !actor.Valid() || expectedRevision < 1 || !identity.ValidID(reviewer) {
		return identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var isolation string
	if err := tx.QueryRow(ctx, "SHOW transaction_isolation").Scan(&isolation); err != nil {
		return storageError{err}
	}
	if isolation != "read committed" {
		return identity.ErrInvalid
	}
	q := identitydb.New(tx)
	if err := lockRecovery(ctx, q, actor.RecoveryID); err != nil {
		return err
	}
	first, second := actor.SubjectID, reviewer
	if second < first {
		first, second = second, first
	}
	if err := lockPublicationAccount(ctx, q, first, actor); err != nil {
		return err
	}
	if second != first {
		if err := lockPublicationAccount(ctx, q, second, actor); err != nil {
			return err
		}
	}
	role, err := readPublisher(ctx, q, actor.SubjectID)
	if errors.Is(err, identity.ErrNotFound) || (err == nil && !role.Enabled) {
		return identity.ErrPublisherRequired
	}
	if err != nil {
		return err
	}
	if role.Revision != expectedRevision {
		return identity.ErrConflict
	}
	return ctx.Err()
}

func lockPublicationAccount(ctx context.Context, q *identitydb.Queries, subject string, actor identity.ProblemActor) error {
	row, err := q.GetStateForShare(ctx, subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrConflict
	}
	if err != nil {
		return storageError{err}
	}
	if row.ID != subject || !row.Enabled || (subject == actor.SubjectID && row.AuthGeneration != actor.AuthGeneration) {
		return identity.ErrConflict
	}
	return nil
}

// PublisherRole reads maintenance metadata without granting publish access.
func (s *ProblemAccess) PublisherRole(ctx context.Context, subject string) (identity.ProblemPublisherRole, error) {
	if s == nil || s.pool == nil || ctx == nil || !identity.ValidID(subject) {
		return identity.ProblemPublisherRole{}, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return readPublisher(ctx, identitydb.New(s.pool), subject)
}

func readPublisher(ctx context.Context, q *identitydb.Queries, subject string) (identity.ProblemPublisherRole, error) {
	row, err := q.LatestProblemPublisher(ctx, subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ProblemPublisherRole{}, identity.ErrNotFound
	}
	if err != nil {
		return identity.ProblemPublisherRole{}, storageError{err}
	}
	if row.SubjectID != subject || row.Revision < 1 {
		return identity.ProblemPublisherRole{}, storageError{identity.ErrInvalid}
	}
	return identity.ProblemPublisherRole{SubjectID: row.SubjectID, Enabled: row.Enabled, Revision: row.Revision}, nil
}

// SetPublisherRole appends an audited maintenance CAS change. Inspect the
// stored revision after an unknown outcome; do not blindly send a new change.
func (s *ProblemAccess) SetPublisherRole(ctx context.Context, change identity.ProblemRoleChange) (identity.ProblemPublisherRole, error) {
	var zero identity.ProblemPublisherRole
	if s == nil || s.pool == nil || ctx == nil || !change.Valid() || change.ExpectedRevision == math.MaxInt64 {
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
	account, err := q.AccountForRoleChange(ctx, change.SubjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, identity.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	if change.Enabled && !account.Enabled {
		return zero, identity.ErrPublisherRequired
	}
	current, err := readPublisher(ctx, q, change.SubjectID)
	if err != nil && !errors.Is(err, identity.ErrNotFound) {
		return zero, err
	}
	if current.Revision != change.ExpectedRevision {
		return zero, identity.ErrConflict
	}
	next := identity.ProblemPublisherRole{SubjectID: change.SubjectID, Enabled: change.Enabled, Revision: change.ExpectedRevision + 1}
	if err := q.AppendProblemPublisher(ctx, identitydb.AppendProblemPublisherParams{SubjectID: next.SubjectID,
		Revision: next.Revision, Enabled: next.Enabled, MaintenanceOperator: change.Operator, Reason: change.Reason}); err != nil {
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

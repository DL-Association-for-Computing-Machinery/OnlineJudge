package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProblemAccess owns the narrow author role and its maintenance audit. It has
// no automatic grants, HTTP administration, or implicit access to others' work.
type ProblemAccess struct{ pool *pgxpool.Pool }

// NewProblemAccess binds administrative operations to a caller-owned pool.
func NewProblemAccess(pool *pgxpool.Pool) (*ProblemAccess, error) {
	if pool == nil {
		return nil, identity.ErrInvalid
	}
	return &ProblemAccess{pool: pool}, nil
}

// LockAuthor checks identity-owned facts in the caller's SAME business tx. The
// caller must use READ COMMITTED and keep the transaction alive through its
// business commit; no filesystem work, Redis calls or responses run under locks.
// Lock order is recovery -> account -> role -> caller's business rows.
func (s *ProblemAccess) LockAuthor(ctx context.Context, tx pgx.Tx, actor identity.ProblemActor) error {
	if s == nil || s.pool == nil || ctx == nil || tx == nil || !actor.Valid() {
		return identity.ErrInvalid
	}
	q := identitydb.New(tx)
	if err := lockRecovery(ctx, q, actor.RecoveryID); err != nil {
		return err
	}
	row, err := q.GetStateForShare(ctx, actor.SubjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrConflict
	}
	if err != nil {
		return storageError{err}
	}
	if row.ID != actor.SubjectID || !row.Enabled || row.AuthGeneration != actor.AuthGeneration {
		return identity.ErrConflict
	}
	role, err := q.ProblemAuthorForShare(ctx, actor.SubjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrForbidden
	}
	if err != nil {
		return storageError{err}
	}
	if role.SubjectID != actor.SubjectID || role.Revision < 1 {
		return storageError{identity.ErrInvalid}
	}
	if !role.Enabled {
		return identity.ErrForbidden
	}
	return ctx.Err()
}

// AuthorRole reads point-in-time maintenance metadata, never a permission grant.
func (s *ProblemAccess) AuthorRole(ctx context.Context, id string) (identity.ProblemAuthorRole, error) {
	if s == nil || s.pool == nil || ctx == nil || !identity.ValidID(id) {
		return identity.ProblemAuthorRole{}, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := identitydb.New(s.pool).ProblemAuthor(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ProblemAuthorRole{}, identity.ErrNotFound
	}
	if err != nil {
		return identity.ProblemAuthorRole{}, storageError{err}
	}
	return identity.ProblemAuthorRole{SubjectID: r.SubjectID, Enabled: r.Enabled, Revision: r.Revision}, nil
}

// SetAuthorRole is trusted maintenance only. Initial expected revision is zero;
// every change and its audit commit atomically. Unknown commit results require
// inspection; a conflict is not permission to blindly advance the revision.
func (s *ProblemAccess) SetAuthorRole(ctx context.Context, c identity.ProblemRoleChange) (identity.ProblemAuthorRole, error) {
	if s == nil || s.pool == nil || ctx == nil || !c.Valid() {
		return identity.ProblemAuthorRole{}, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return identity.ProblemAuthorRole{}, storageError{err}
	}
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(clean)
	}()
	q := identitydb.New(tx)
	account, err := q.AccountForRoleChange(ctx, c.SubjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ProblemAuthorRole{}, identity.ErrNotFound
	}
	if err != nil {
		return identity.ProblemAuthorRole{}, storageError{err}
	}
	if c.Enabled && !account.Enabled {
		return identity.ProblemAuthorRole{}, identity.ErrForbidden
	}
	old, err := q.ProblemAuthorForUpdate(ctx, c.SubjectID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return identity.ProblemAuthorRole{}, storageError{err}
	}
	if old.Revision != c.ExpectedRevision {
		return identity.ProblemAuthorRole{}, identity.ErrConflict
	}
	var role identitydb.IdentityProblemAuthor
	if c.ExpectedRevision == 0 {
		role, err = q.CreateProblemAuthor(ctx, identitydb.CreateProblemAuthorParams{SubjectID: c.SubjectID, Enabled: c.Enabled})
	} else {
		role, err = q.ChangeProblemAuthor(ctx, identitydb.ChangeProblemAuthorParams{SubjectID: c.SubjectID, Enabled: c.Enabled, ExpectedRevision: c.ExpectedRevision})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ProblemAuthorRole{}, identity.ErrConflict
	}
	if err != nil {
		return identity.ProblemAuthorRole{}, storageError{err}
	}
	if err := q.AuditProblemAuthor(ctx, identitydb.AuditProblemAuthorParams{SubjectID: c.SubjectID, PreviousRevision: c.ExpectedRevision, Revision: role.Revision, Enabled: c.Enabled, MaintenanceOperator: c.Operator, Reason: c.Reason}); err != nil {
		return identity.ProblemAuthorRole{}, storageError{err}
	}
	if err := tx.Commit(ctx); err != nil {
		return identity.ProblemAuthorRole{}, storageError{err}
	}
	return identity.ProblemAuthorRole{SubjectID: role.SubjectID, Enabled: role.Enabled, Revision: role.Revision}, nil
}

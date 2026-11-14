package postgres

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PlatformAdmins owns the platform role history. It does not issue sessions or
// grant a business role such as problem author, reviewer or contest organizer.
type PlatformAdmins struct{ pool *pgxpool.Pool }

// NewPlatformAdmins binds maintenance to a caller-owned pool.
func NewPlatformAdmins(pool *pgxpool.Pool) (*PlatformAdmins, error) {
	if pool == nil {
		return nil, identity.ErrInvalid
	}
	return &PlatformAdmins{pool: pool}, nil
}

func readPlatformAdmin(ctx context.Context, q *identitydb.Queries, subject string) (identity.PlatformAdminRole, error) {
	row, err := q.LatestPlatformAdmin(ctx, subject)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return identity.PlatformAdminRole{}, identity.ErrNotFound
	case err != nil:
		return identity.PlatformAdminRole{}, storageError{err}
	case row.SubjectID != subject || row.Revision < 1:
		return identity.PlatformAdminRole{}, storageError{identity.ErrInvalid}
	default:
		return identity.PlatformAdminRole{SubjectID: subject, Revision: row.Revision, Enabled: row.Enabled}, nil
	}
}

// Inspect returns the latest revision, not an authorization decision.
func (s *PlatformAdmins) Inspect(ctx context.Context, subject string) (identity.PlatformAdminRole, error) {
	if s == nil || s.pool == nil || ctx == nil || !identity.ValidID(subject) {
		return identity.PlatformAdminRole{}, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return readPlatformAdmin(ctx, identitydb.New(s.pool), subject)
}

// Change appends one audited CAS revision. The account lock serializes both
// first-grant races and admission against changes to an existing grant.
func (s *PlatformAdmins) Change(ctx context.Context, c identity.PlatformAdminChange) (identity.PlatformAdminRole, error) {
	var zero identity.PlatformAdminRole
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
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
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
		return zero, identity.ErrPlatformAdminRequired
	}
	if c.Enabled {
		if err := requireVerifiedAdminEmail(ctx, q, c.SubjectID); err != nil {
			return zero, err
		}
	}
	old, err := readPlatformAdmin(ctx, q, c.SubjectID)
	if err != nil && !errors.Is(err, identity.ErrNotFound) {
		return zero, err
	}
	if old.Revision != c.ExpectedRevision {
		return zero, identity.ErrConflict
	}
	next := identity.PlatformAdminRole{SubjectID: c.SubjectID, Revision: old.Revision + 1, Enabled: c.Enabled}
	if err := q.AppendPlatformAdmin(ctx, identitydb.AppendPlatformAdminParams{SubjectID: next.SubjectID,
		Revision: next.Revision, Enabled: next.Enabled, MaintenanceOperator: c.Operator, Reason: c.Reason}); err != nil {
		return zero, storageError{err}
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, storageError{err}
	}
	return next, nil
}

// LockPlatformAdmin rechecks current authority in a caller-owned READ COMMITTED
// business transaction. Call before locking resources; on failure roll back.
// The account share lock prevents role changes until that transaction ends.
func LockPlatformAdmin(ctx context.Context, tx pgx.Tx, actor identity.PlatformAdminActor) error {
	if ctx == nil || tx == nil || !actor.Valid() {
		return identity.ErrInvalid
	}
	// A frozen snapshot could predate a role append even after the account
	// lock is acquired. Refuse any isolation level without fresh snapshots.
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
	state, err := q.GetStateForShare(ctx, actor.SubjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrConflict
	}
	if err != nil {
		return storageError{err}
	}
	if state.ID != actor.SubjectID || !state.Enabled || state.AuthGeneration != actor.AuthGeneration {
		return identity.ErrConflict
	}
	if err := requireVerifiedAdminEmail(ctx, q, actor.SubjectID); err != nil {
		return err
	}
	role, err := readPlatformAdmin(ctx, q, actor.SubjectID)
	if errors.Is(err, identity.ErrNotFound) {
		return identity.ErrPlatformAdminRequired
	}
	if err != nil {
		return err
	}
	if !role.Enabled || role.Revision != actor.RoleRevision {
		return identity.ErrPlatformAdminRequired
	}
	return ctx.Err()
}

func requireVerifiedAdminEmail(ctx context.Context, q *identitydb.Queries, subject string) error {
	verified, err := q.TrainingEmailForShare(ctx, subject)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return identity.ErrPlatformAdminRequired
	case err != nil:
		return storageError{err}
	case !verified:
		return identity.ErrPlatformAdminRequired
	default:
		return nil
	}
}

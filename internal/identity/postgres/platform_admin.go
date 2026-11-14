package postgres

import (
	"context"
	"errors"
	"math"
	"strings"
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
	if s == nil || s.pool == nil || ctx == nil || !c.Valid() || c.ExpectedRevision == math.MaxInt64 || strings.HasPrefix(c.Operator, identity.PlatformAdminOperatorPrefix) {
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
	next, err := appendPlatformAdmin(ctx, q, c, account.Enabled, nil)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, storageError{err}
	}
	return next, nil
}

func appendPlatformAdmin(ctx context.Context, q *identitydb.Queries, c identity.PlatformAdminChange, enabled bool, confirm func(context.Context) error) (identity.PlatformAdminRole, error) {
	var zero identity.PlatformAdminRole
	if c.Enabled && !enabled {
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
	if confirm != nil {
		if err := confirm(ctx); err != nil {
			return zero, err
		}
	}
	next := identity.PlatformAdminRole{SubjectID: c.SubjectID, Revision: old.Revision + 1, Enabled: c.Enabled}
	if err := q.AppendPlatformAdmin(ctx, identitydb.AppendPlatformAdminParams{SubjectID: next.SubjectID,
		Revision: next.Revision, Enabled: next.Enabled, MaintenanceOperator: c.Operator, Reason: c.Reason}); err != nil {
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
	_, err := lockPlatformAdmin(ctx, identitydb.New(tx), actor.SubjectID, actor.SessionVersion, 0, actor.RoleRevision)
	return err
}

// WithAdminSession coordinates a trusted short storage callback under the
// current role and identity locks. It never verifies a password or a token.
func (s *PlatformAdmins) WithAdminSession(ctx context.Context, actor identity.PlatformAdminActor, operation func(context.Context) error) error {
	if !actor.Valid() || operation == nil {
		return identity.ErrInvalid
	}
	return s.withSession(ctx, actor.SubjectID, actor.SessionVersion, 0, actor.RoleRevision, func(call context.Context, _ identity.PlatformAdminActor) error {
		return operation(call)
	})
}

// WithAdminAdmission rechecks the exact password verification and the role
// observed before verification. The callback must not run password work or
// publish a cookie. It may have written externally even if this returns error.
func (s *PlatformAdmins) WithAdminAdmission(ctx context.Context, checked identity.SessionState, roleRevision int64, operation func(context.Context, identity.PlatformAdminActor) error) error {
	if !identity.ValidID(checked.ID) || !checked.Valid() || !checked.Enabled ||
		!identity.ValidRecoveryID(checked.RecoveryID) || roleRevision <= 0 || operation == nil {
		return identity.ErrInvalid
	}
	return s.withSession(ctx, checked.ID, identity.SessionVersion{AuthGeneration: checked.AuthGeneration, RecoveryID: checked.RecoveryID}, checked.CredentialVersion, roleRevision, operation)
}

func (s *PlatformAdmins) withSession(ctx context.Context, subject string, version identity.SessionVersion, credentialVersion, roleRevision int64, operation func(context.Context, identity.PlatformAdminActor) error) error {
	if s == nil || s.pool == nil || ctx == nil {
		return identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return storageError{err}
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	actor, err := lockPlatformAdmin(ctx, identitydb.New(tx), subject, version, credentialVersion, roleRevision)
	if err != nil {
		return err
	}
	if err := operation(ctx, actor); err != nil {
		return guardedError{err}
	}
	if err := ctx.Err(); err != nil {
		return guardedError{err}
	}
	if err := tx.Commit(ctx); err != nil {
		return guardedError{storageError{err}}
	}
	return nil
}

func lockPlatformAdmin(ctx context.Context, q *identitydb.Queries, subject string, version identity.SessionVersion, credentialVersion, roleRevision int64) (identity.PlatformAdminActor, error) {
	var zero identity.PlatformAdminActor
	if err := lockRecovery(ctx, q, version.RecoveryID); err != nil {
		return zero, err
	}
	state, err := q.GetStateForShare(ctx, subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, identity.ErrConflict
	}
	if err != nil {
		return zero, storageError{err}
	}
	if state.ID != subject || !state.Enabled || state.AuthGeneration != version.AuthGeneration ||
		(credentialVersion > 0 && state.CredentialVersion != credentialVersion) {
		return zero, identity.ErrConflict
	}
	if err := requireVerifiedAdminEmail(ctx, q, subject); err != nil {
		return zero, err
	}
	role, err := readPlatformAdmin(ctx, q, subject)
	if errors.Is(err, identity.ErrNotFound) {
		return zero, identity.ErrPlatformAdminRequired
	}
	if err != nil {
		return zero, err
	}
	if !role.Enabled || role.Revision != roleRevision {
		return zero, identity.ErrPlatformAdminRequired
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return identity.PlatformAdminActor{SubjectID: subject, SessionVersion: version, RoleRevision: role.Revision}, nil
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

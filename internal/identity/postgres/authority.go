package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// WithCredentials coordinates a trusted, short session-storage operation with
// account changes. It checks both revisions but is only a subject-state primitive,
// never a standalone formal-session gate; use WithSessionCredentials for that scope.
// It does not verify passwords, activation, resource permissions or recovery.
// Operation follows WithAccount's restrictions and uncertain-outcome contract.
func (s *Store) WithCredentials(ctx context.Context, id string, want identity.Revision, operation func(context.Context, identity.State) error) error {
	if !want.Valid() {
		return identity.ErrInvalid
	}
	return s.withAuthority(ctx, id, want, true, subjectScope, "", operation)
}

// WithAccount only coordinates subject enabled state and auth generation.
// It is not a standalone formal-session gate; session stores must use WithSession.
// It intentionally ignores credential version: same-password rehashing does not
// revoke sessions. Both methods invoke operation synchronously at most once.
//
// Operations receive a cooperative five-second context, not a hard interrupt.
// They must honor it, avoid KDF, reentry, escaped tasks and HTTP/Cookie publication.
// A lost database connection may release the lock while operation is still
// running. Session candidates must carry the checked subject/auth generation;
// later use must revalidate authority. This cannot undo external effects and
// does not establish a PostgreSQL/Redis transaction or a complete session gate.
func (s *Store) WithAccount(ctx context.Context, id string, authGeneration int64, operation func(context.Context, identity.State) error) error {
	if authGeneration <= 0 {
		return identity.ErrInvalid
	}
	return s.withAuthority(ctx, id, identity.Revision{AuthGeneration: authGeneration}, false, subjectScope, "", operation)
}

func (s *Store) withAuthority(ctx context.Context, id string, want identity.Revision, credentials bool, scope authorityScope, recoveryID string, operation func(context.Context, identity.State) error) error {
	if s == nil || s.pool == nil || ctx == nil || !identity.ValidID(id) || operation == nil {
		return identity.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return storageError{err}
	}
	defer func() {
		// Rollback is best effort even on panic/cancellation. pgx discards a
		// broken connection; cleanup cannot compensate an external operation.
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	q := s.q.WithTx(tx)
	if scope == sessionScope {
		if !identity.ValidRecoveryID(recoveryID) {
			return identity.ErrInvalid
		}
		if err := lockRecovery(ctx, q, recoveryID); err != nil {
			return err
		}
	}
	row, err := q.GetStateForShare(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrConflict
	}
	if err != nil {
		return storageError{err}
	}
	state := identity.State{ID: row.ID, Enabled: row.Enabled, Revision: identity.Revision{
		CredentialVersion: row.CredentialVersion, AuthGeneration: row.AuthGeneration,
	}}
	if state.ID != id || !state.Valid() {
		return storageError{identity.ErrInvalid}
	}
	if !state.Enabled || state.AuthGeneration != want.AuthGeneration || (credentials && state.CredentialVersion != want.CredentialVersion) {
		return identity.ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := operation(ctx, state); err != nil {
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

// Keep callback details out of Error while preserving diagnostic classifications.
type guardedError struct{ cause error }

func (e guardedError) Error() string { return identity.ErrGuardedOperation.Error() }
func (e guardedError) Unwrap() error { return e.cause }
func (e guardedError) Is(target error) bool {
	return target == identity.ErrGuardedOperation
}

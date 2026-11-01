package postgres

import (
	"context"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
)

// ReplacePasswordForSession replaces a verified current password and revokes
// old session generations in one transaction. Recovery is locked first; the
// conditional UPDATE then acquires the account write lock without upgrading a
// prior account share lock. Password work and session lifetime checks belong to
// the caller. A commit error can mean success with a lost receipt; never retry.
func (s *Store) ReplacePasswordForSession(ctx context.Context, id string, want identity.Revision, recoveryID string, record password.Record) (identity.Snapshot, error) {
	if s == nil || s.pool == nil || s.q == nil || s.hasher == nil || ctx == nil || !want.Valid() || !identity.ValidRecoveryID(recoveryID) {
		return identity.Snapshot{}, identity.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return identity.Snapshot{}, err
	}
	if want.CredentialVersion == math.MaxInt64 || want.AuthGeneration == math.MaxInt64 {
		return identity.Snapshot{}, identity.ErrConflict
	}
	if err := s.checkWrite(id, record); err != nil {
		return identity.Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return identity.Snapshot{}, storageError{err}
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	q := s.q.WithTx(tx)
	if err := lockRecovery(ctx, q, recoveryID); err != nil {
		return identity.Snapshot{}, err
	}
	row, err := q.ReplacePasswordForSession(ctx, identitydb.ReplacePasswordForSessionParams(writeParams(id, want, record)))
	changed, err := s.writeResult(row, err)
	if err != nil {
		return identity.Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return identity.Snapshot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return identity.Snapshot{}, storageError{err}
	}
	return changed, nil
}

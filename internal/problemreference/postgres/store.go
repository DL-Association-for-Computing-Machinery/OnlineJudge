// Package postgres owns bounded, authorized reference-request transactions.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DraftAccess is implemented by problem's public adapter. Every authority and
// draft check uses the caller's SAME tx, without importing private module SQL.
type DraftAccess interface {
	LockAuthorSnapshot(context.Context, pgx.Tx, identity.ProblemActor) (identity.ProblemAuthorSnapshot, error)
	LockOwnDraftIdentity(context.Context, pgx.Tx, identity.ProblemActor, string) (problem.DraftIdentity, error)
}

// Store owns no pool lifecycle. Calls use a five-second context budget plus
// independent best-effort rollback; they cannot prove a remote tx has ended.
type Store struct {
	pool   *pgxpool.Pool
	access DraftAccess
}

var _ problemreference.Repository = (*Store)(nil)

// New requires current same-transaction identity/draft access.
func New(pool *pgxpool.Pool, access DraftAccess) (*Store, error) {
	if pool == nil || access == nil {
		return nil, problemreference.ErrInvalid
	}
	return &Store{pool: pool, access: access}, nil
}

type storageError struct{ cause error }

func (e storageError) Error() string        { return problemreference.ErrStore.Error() }
func (e storageError) Unwrap() error        { return e.cause }
func (e storageError) Is(target error) bool { return target == problemreference.ErrStore }

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func (s *Store) begin(ctx context.Context, actor identity.ProblemActor) (pgx.Tx, identity.ProblemAuthorSnapshot, error) {
	if s == nil || s.pool == nil || ctx == nil || !actor.Valid() {
		return nil, identity.ProblemAuthorSnapshot{}, problemreference.ErrInvalid
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, identity.ProblemAuthorSnapshot{}, storageError{err}
	}
	authority, err := s.access.LockAuthorSnapshot(ctx, tx, actor)
	if err != nil {
		rollback(tx)
		return nil, identity.ProblemAuthorSnapshot{}, err
	}
	if authority.Actor != actor || authority.RoleRevision < 1 {
		rollback(tx)
		return nil, identity.ProblemAuthorSnapshot{}, storageError{problemreference.ErrInvalid}
	}
	return tx, authority, nil
}

func commit(ctx context.Context, tx pgx.Tx, result problemreference.Request, wrote bool) (problemreference.Request, error) {
	if err := commitTx(ctx, tx, wrote); err != nil {
		return problemreference.Request{}, err
	}
	return result, nil
}

func commitTx(ctx context.Context, tx pgx.Tx, wrote bool) error {
	if err := tx.Commit(ctx); err != nil {
		if wrote && !errors.Is(err, pgx.ErrTxCommitRollback) {
			return errors.Join(problemreference.ErrCommitUnknown, storageError{err})
		}
		return storageError{err}
	}
	return nil
}

// Configure changes only the explicit queued-request policy. It is a trusted
// maintenance operation, not called by API startup; lowering never deletes work.
func Configure(ctx context.Context, pool *pgxpool.Pool, limits problemreference.Limits) error {
	if ctx == nil || pool == nil || !limits.Valid() {
		return problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := referencedb.New(pool).SetReferencePolicy(ctx, referencedb.SetReferencePolicyParams{GlobalLimit: limits.Global, OwnerLimit: limits.Owner})
	if err != nil {
		return storageError{err}
	}
	return nil
}

func lockPolicy(ctx context.Context, q *referencedb.Queries) (problemreference.Limits, error) {
	row, err := q.LockReferencePolicy(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return problemreference.Limits{}, problemreference.ErrPolicy
	}
	if err != nil {
		return problemreference.Limits{}, storageError{err}
	}
	l := problemreference.Limits{Global: row.GlobalLimit, Owner: row.OwnerLimit}
	if !l.Valid() {
		return problemreference.Limits{}, storageError{problemreference.ErrInvalid}
	}
	return l, nil
}

func readRequest(row referencedb.ProblemReferenceRequest) (problemreference.Request, error) {
	if !identity.ValidID(row.ID) || !identity.ValidID(row.OwnerSubjectID) || !problem.ValidKey(row.RequestKey) || !identity.ValidID(row.DraftID) || !row.CreatedAt.Valid || row.CreatedAt.InfinityModifier != 0 || row.CancelledAt.InfinityModifier != 0 || row.CreatedAt.Time.Year() < 1 || row.CreatedAt.Time.Year() > 9999 || (row.CancelledAt.Valid && (row.CancelledAt.Time.Year() < 1 || row.CancelledAt.Time.Year() > 9999)) {
		return problemreference.Request{}, storageError{problemreference.ErrInvalid}
	}
	spec := problemreference.Specification{Identity: problem.ImportIdentity{SourceID: row.SourceID, Version: row.ContentVersion, ImportRevision: row.ImportRevision, SourceLockSHA256: row.SourceLockSha256},
		Protocol: row.IdentityProtocol, ProfileJSON: append([]byte(nil), row.ProfileEncoding...), ProfileSHA256: row.ProfileSha256}
	authority := identity.ProblemAuthorSnapshot{Actor: identity.ProblemActor{SubjectID: row.OwnerSubjectID, SessionVersion: identity.SessionVersion{AuthGeneration: row.AuthGeneration, RecoveryID: row.RecoveryID}}, RoleRevision: row.AuthorRevision}
	if !spec.Valid() || !authority.Actor.Valid() || authority.RoleRevision < 1 || (row.State != problemreference.Queued && row.State != problemreference.Cancelled) || row.CancelledAt.Valid != (row.State == problemreference.Cancelled) {
		return problemreference.Request{}, storageError{problemreference.ErrInvalid}
	}
	result := problemreference.Request{ID: row.ID, Owner: row.OwnerSubjectID, RequestKey: row.RequestKey, DraftID: row.DraftID,
		Specification: spec, Authorization: authority, State: row.State, CreatedAt: row.CreatedAt.Time}
	if row.CancelledAt.Valid {
		t := row.CancelledAt.Time
		result.CancelledAt = &t
	}
	return result, nil
}

func existing(ctx context.Context, q *referencedb.Queries, actor identity.ProblemActor, key, draftID string) (problemreference.Request, bool, error) {
	row, err := q.ReferenceRequest(ctx, referencedb.ReferenceRequestParams{Owner: actor.SubjectID, RequestKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return problemreference.Request{}, false, nil
	}
	if err != nil {
		return problemreference.Request{}, false, storageError{err}
	}
	if draftID != "" && row.DraftID != draftID {
		return problemreference.Request{}, true, problemreference.ErrConflict
	}
	result, err := readRequest(row)
	return result, true, err
}

// Package postgres persists identity-owned credential state in PostgreSQL.
// It has no public registration, password derivation, or session side effects.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
)

// Store uses a caller-owned bounded pool and immutable password policy.
// A call has a five-second context ceiling; the caller may set a shorter one.
// Database failures remain uncertain outcomes and must not be blindly retried.
type Store struct {
	q      *identitydb.Queries
	hasher *password.Hasher
}

// New checks that every approved record fits the storage envelope. It neither
// opens nor closes the pool, migrates the schema, nor selects a KDF policy.
func New(pool *pgxpool.Pool, hasher *password.Hasher) (*Store, error) {
	salt, digest := hasher.RecordSizeBounds()
	if pool == nil || salt == 0 || digest == 0 || salt > identity.StoredSaltBytes || digest > identity.StoredDigestBytes {
		return nil, identity.ErrInvalid
	}
	return &Store{q: identitydb.New(pool), hasher: hasher}, nil
}

// Initialize inserts one new, non-reusable subject ID with revisions 1/1.
// It accepts only Current records and never overwrites an existing subject.
// Enabled is explicit and does not confer activation or resource permissions.
func (s *Store) Initialize(ctx context.Context, id string, enabled bool, record password.Record) (identity.Snapshot, error) {
	if err := s.checkWrite(id, record); err != nil {
		return identity.Snapshot{}, err
	}
	p := writeParams(id, identity.Revision{}, record)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := s.q.Initialize(ctx, identitydb.InitializeParams{
		ID: id, Enabled: enabled, FormatVersion: p.FormatVersion, Algorithm: p.Algorithm,
		ArgonVersion: p.ArgonVersion, MemoryKib: p.MemoryKib, Iterations: p.Iterations,
		Parallelism: p.Parallelism, SaltBytes: p.SaltBytes, DigestBytes: p.DigestBytes,
		Salt: p.Salt, Digest: p.Digest,
	})
	return s.writeResult(row, err)
}

// Get returns an internal snapshot, allowing both approved Current and Legacy
// records. A successful read is not a durable login or authorization decision.
func (s *Store) Get(ctx context.Context, id string) (identity.Snapshot, error) {
	if !identity.ValidID(id) {
		return identity.Snapshot{}, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := s.q.Get(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Snapshot{}, identity.ErrNotFound
	}
	if err != nil {
		return identity.Snapshot{}, storageError{err}
	}
	return s.snapshot(row)
}

// State reads only subject state and revisions, so a fresh caller can prepare
// revocation even when Get rejects retired password material. Like Get, it does
// not lock future changes or itself authorize any account-management action.
func (s *Store) State(ctx context.Context, id string) (identity.State, error) {
	if !identity.ValidID(id) {
		return identity.State{}, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := s.q.GetState(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.State{}, identity.ErrNotFound
	}
	if err != nil {
		return identity.State{}, storageError{err}
	}
	rev := identity.Revision{CredentialVersion: row.CredentialVersion, AuthGeneration: row.AuthGeneration}
	if row.ID != id || !rev.Valid() {
		return identity.State{}, storageError{identity.ErrInvalid}
	}
	return identity.State{ID: row.ID, Enabled: row.Enabled, Revision: rev}, nil
}

func (s *Store) checkWrite(id string, record password.Record) error {
	if !identity.ValidID(id) {
		return identity.ErrInvalid
	}
	class, err := s.hasher.CheckRecord(record)
	if err != nil {
		return err
	}
	if class != password.CurrentRecord {
		return password.ErrRecord
	}
	return nil
}

func (s *Store) writeResult(row identitydb.IdentityCredential, err error) (identity.Snapshot, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Snapshot{}, identity.ErrConflict
	}
	if err != nil {
		return identity.Snapshot{}, storageError{err}
	}
	result, err := s.snapshot(row)
	if err != nil {
		// A malformed RETURNING row does not prove the write was rolled back.
		return identity.Snapshot{}, storageError{err}
	}
	return result, nil
}

// Keep the driver cause available to internal diagnostics while Error never
// echoes PostgreSQL details, which can contain a complete failing record.
type storageError struct{ cause error }

func (e storageError) Error() string        { return identity.ErrStore.Error() }
func (e storageError) Unwrap() error        { return e.cause }
func (e storageError) Is(target error) bool { return target == identity.ErrStore }

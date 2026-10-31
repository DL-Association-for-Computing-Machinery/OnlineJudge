package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
)

// RehashCAS is a trusted persistence primitive, not a login/upgrade service.
// The caller must first verify the old snapshot and create Current using the
// same password outside database locks. This method cannot prove same-password
// equivalence. It changes only credential version and requires enabled state.
func (s *Store) RehashCAS(ctx context.Context, id string, want identity.Revision, record password.Record) (identity.Snapshot, error) {
	if !want.Valid() {
		return identity.Snapshot{}, identity.ErrInvalid
	}
	if err := s.checkWrite(id, record); err != nil {
		return identity.Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := s.q.RehashCAS(ctx, writeParams(id, want, record))
	return s.writeResult(row, err)
}

// ReplacePasswordCAS replaces the record and advances both revisions. It never
// enables a disabled account. Authorization and one-time reset-token handling
// belong to the future application workflow, not this storage operation.
func (s *Store) ReplacePasswordCAS(ctx context.Context, id string, want identity.Revision, record password.Record) (identity.Snapshot, error) {
	if !want.Valid() {
		return identity.Snapshot{}, identity.ErrInvalid
	}
	if err := s.checkWrite(id, record); err != nil {
		return identity.Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := s.q.ReplacePasswordCAS(ctx, identitydb.ReplacePasswordCASParams(writeParams(id, want, record)))
	return s.writeResult(row, err)
}

// DisableCAS closes an enabled account and advances only auth generation.
// It intentionally does not decode/validate the old record, so corrupt or
// retired password material cannot prevent revocation. It does not clean Redis.
func (s *Store) DisableCAS(ctx context.Context, id string, want identity.Revision) (identity.Revision, error) {
	if !identity.ValidID(id) || !want.Valid() {
		return identity.Revision{}, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := s.q.DisableCAS(ctx, identitydb.DisableCASParams{
		ID: id, ExpectedCredentialVersion: want.CredentialVersion, ExpectedAuthGeneration: want.AuthGeneration,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Revision{}, identity.ErrConflict
	}
	if err != nil {
		return identity.Revision{}, storageError{err}
	}
	return identity.Revision{CredentialVersion: row.CredentialVersion, AuthGeneration: row.AuthGeneration}, nil
}

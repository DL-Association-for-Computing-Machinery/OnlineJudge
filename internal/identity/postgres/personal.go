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

// InitializePersonal atomically inserts a new subject and its unverified email.
// The Current record must be derived before calling; no KDF runs in the transaction.
// Existing IDs or email keys conflict without replacing or attaching identities.
// This trusted primitive is not public signup; errors may have unknown outcomes.
func (s *Store) InitializePersonal(ctx context.Context, id string, email identity.Email, enabled bool, record password.Record) (identity.Personal, error) {
	if err := s.checkPersonal(ctx); err != nil {
		return identity.Personal{}, err
	}
	if !email.Valid() {
		return identity.Personal{}, identity.ErrInvalid
	}
	if err := s.checkWrite(id, record); err != nil {
		return identity.Personal{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return identity.Personal{}, storageError{err}
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	q := s.q.WithTx(tx)
	row, err := q.Initialize(ctx, initializeParams(id, enabled, record))
	snapshot, err := s.writeResult(row, err)
	if err != nil {
		return identity.Personal{}, err
	}
	mapping, err := q.InitializePersonalEmail(ctx, identitydb.InitializePersonalEmailParams{
		SubjectID: id, EmailKey: email.Key(), OriginalEmail: email.Original(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Personal{}, identity.ErrConflict
	}
	if err != nil {
		return identity.Personal{}, storageError{err}
	}
	if mapping.SubjectID != id || mapping.EmailKey != email.Key() || mapping.OriginalEmail != email.Original() || mapping.EmailVerified {
		return identity.Personal{}, storageError{identity.ErrInvalid}
	}
	if err := tx.Commit(ctx); err != nil {
		return identity.Personal{}, storageError{err}
	}
	return identity.Personal{State: snapshot.State, Email: email}, nil
}

// PersonalByID reads non-password metadata. It does not authorize a session or
// training access, and returns ErrNotFound for generic subjects without email.
func (s *Store) PersonalByID(ctx context.Context, id string) (identity.Personal, error) {
	if err := s.checkPersonal(ctx); err != nil {
		return identity.Personal{}, err
	}
	if !identity.ValidID(id) {
		return identity.Personal{}, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := s.q.PersonalByID(ctx, id)
	if err != nil {
		return identity.Personal{}, personalReadError(err)
	}
	result, err := personalResult(row.ID, row.Enabled, row.CredentialVersion, row.AuthGeneration, row.EmailKey, row.OriginalEmail, row.EmailVerified)
	if err == nil && result.ID != id {
		return identity.Personal{}, storageError{identity.ErrInvalid}
	}
	return result, err
}

// LookupPersonalByEmail resolves a validated email key, never a nickname or an
// opaque subject ID supplied as a personal login name. Reads are point-in-time.
func (s *Store) LookupPersonalByEmail(ctx context.Context, email identity.Email) (identity.Personal, error) {
	if err := s.checkPersonal(ctx); err != nil {
		return identity.Personal{}, err
	}
	if !email.Valid() {
		return identity.Personal{}, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := s.q.PersonalByEmail(ctx, email.Key())
	if err != nil {
		return identity.Personal{}, personalReadError(err)
	}
	if row.EmailKey != email.Key() {
		return identity.Personal{}, storageError{identity.ErrInvalid}
	}
	return personalResult(row.ID, row.Enabled, row.CredentialVersion, row.AuthGeneration, row.EmailKey, row.OriginalEmail, row.EmailVerified)
}

func (s *Store) checkPersonal(ctx context.Context) error {
	if s == nil || s.pool == nil || s.q == nil || s.hasher == nil || ctx == nil {
		return identity.ErrInvalid
	}
	return ctx.Err()
}

func personalReadError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrNotFound
	}
	return storageError{err}
}

func personalResult(id string, enabled bool, credentialVersion, authGeneration int64, key, original string, verified bool) (identity.Personal, error) {
	email, err := identity.ParseEmail(original)
	rev := identity.Revision{CredentialVersion: credentialVersion, AuthGeneration: authGeneration}
	if err != nil || email.Key() != key || !identity.ValidID(id) || !rev.Valid() {
		return identity.Personal{}, storageError{identity.ErrInvalid}
	}
	return identity.Personal{State: identity.State{ID: id, Enabled: enabled, Revision: rev}, Email: email, EmailVerified: verified}, nil
}

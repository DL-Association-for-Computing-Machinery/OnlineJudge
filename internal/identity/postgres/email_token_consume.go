package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
)

// ActivateEmail consumes an activation credential, installs a password chosen
// by the mailbox owner and revokes any earlier login authority. The caller
// applies password policy and bounded KDF work before calling; no HTTP request
// or session is authorized here. A commit error may have an unknown outcome.
func (s *Store) ActivateEmail(ctx context.Context, token emailtoken.Token, record password.Record) error {
	return s.consumeEmailToken(ctx, emailtoken.Activation, token, record)
}

// ResetPasswordByEmail consumes a recovery credential for a verified email.
// It replaces the password and revokes earlier login authority atomically.
// The caller applies password policy and bounded KDF work; no session is issued.
// A commit error may have an unknown outcome.
func (s *Store) ResetPasswordByEmail(ctx context.Context, token emailtoken.Token, record password.Record) error {
	return s.consumeEmailToken(ctx, emailtoken.Recovery, token, record)
}

func (s *Store) consumeEmailToken(ctx context.Context, purpose emailtoken.Purpose, token emailtoken.Token, record password.Record) error {
	if err := s.checkPersonal(ctx); err != nil {
		return err
	}
	digest, err := token.Digest(purpose)
	if err != nil {
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
	q := s.q.WithTx(tx)
	// Discover the lock keys without taking a token lock before the account.
	// Check the same immutable token again after acquiring authority.
	binding, err := q.EmailTokenIdentity(ctx, identitydb.EmailTokenIdentityParams{Digest: digest[:], Purpose: string(purpose)})
	if err != nil {
		return emailIssueReadError(err)
	}
	if !identity.ValidID(binding.ID) || !identity.ValidRecoveryID(binding.RecoveryID) || binding.AuthGeneration <= 0 {
		return storageError{identity.ErrInvalid}
	}
	if err := s.checkWrite(binding.SubjectID, record); err != nil {
		return err
	}
	if err := lockRecovery(ctx, q, binding.RecoveryID); err != nil {
		return err
	}
	account, err := q.EmailTokenConsumeAccount(ctx, binding.SubjectID)
	if err != nil {
		return emailIssueReadError(err)
	}
	if !account.Enabled || account.AuthGeneration != binding.AuthGeneration {
		return identity.ErrConflict
	}
	email, err := q.EmailForTokenIssue(ctx, binding.SubjectID)
	if err != nil {
		return emailIssueReadError(err)
	}
	if email.EmailVerified != (purpose == emailtoken.Recovery) {
		return identity.ErrConflict
	}
	now, err := q.EmailIssueTime(ctx)
	if err != nil {
		return storageError{err}
	}
	if !now.Valid || now.InfinityModifier != pgtype.Finite {
		return storageError{identity.ErrInvalid}
	}
	current, err := q.EmailTokenCurrent(ctx, identitydb.EmailTokenCurrentParams{
		ID: binding.ID, SubjectID: binding.SubjectID, Digest: digest[:], Purpose: string(purpose),
		AuthGeneration: binding.AuthGeneration, RecoveryID: binding.RecoveryID, CheckedAt: now,
	})
	if err != nil {
		return storageError{err}
	}
	if !current {
		return identity.ErrConflict
	}
	want := identity.Revision{CredentialVersion: account.CredentialVersion, AuthGeneration: account.AuthGeneration}
	changed, err := q.ReplacePasswordForSession(ctx, identitydb.ReplacePasswordForSessionParams(writeParams(binding.SubjectID, want, record)))
	if _, err := s.writeResult(changed, err); err != nil {
		return err
	}
	if purpose == emailtoken.Activation {
		verified, err := q.MarkEmailVerified(ctx, binding.SubjectID)
		if err != nil {
			return storageError{err}
		}
		if verified != 1 {
			return identity.ErrConflict
		}
	}
	if err := q.DeleteLiveEmailTokenPayloads(ctx, identitydb.DeleteLiveEmailTokenPayloadsParams{
		SubjectID: binding.SubjectID, Purpose: string(purpose),
		AuthGeneration: binding.AuthGeneration, RecoveryID: binding.RecoveryID, CheckedAt: now,
	}); err != nil {
		return storageError{err}
	}
	if err := tx.Commit(ctx); err != nil {
		return storageError{err}
	}
	return nil
}

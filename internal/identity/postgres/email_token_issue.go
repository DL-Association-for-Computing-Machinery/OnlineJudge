package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
)

// EmailTokenEnqueue inserts only a payload ID into the caller's transaction.
// The trusted adapter must honor the context and must not commit, perform
// external I/O, retain the transaction or return success without inserting.
type EmailTokenEnqueue func(context.Context, pgx.Tx, string) error

// EmailTokenSealBudget reserves one seal under the active key. The reservation
// must be globally bounded, durable independently of this issuance transaction
// and unrecoverable by reusing a key ID or restoring an older budget. Do not use
// this store's connection pool or acquire account locks in the adapter.
type EmailTokenSealBudget func(context.Context, string) error

// IssueEmailToken stores one token digest and its encrypted mail payload for an
// existing personal account. It does not expose the token or change the account.
// The caller supplies the observed account and recovery versions and must apply
// public admission and response rules before exposing any registration endpoint.
// A storage failure may have an unknown outcome; do not automatically retry it.
func (s *Store) IssueEmailToken(ctx context.Context, subject string, expected identity.SessionVersion, purpose emailtoken.Purpose,
	protector *emailtoken.Protector, reserve EmailTokenSealBudget, enqueue EmailTokenEnqueue,
) (string, error) {
	if err := s.checkPersonal(ctx); err != nil {
		return "", err
	}
	if !identity.ValidID(subject) || !expected.Valid() || purpose.Lifetime() == 0 || protector.CurrentKeyID() == "" ||
		reserve == nil || enqueue == nil {
		return "", identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return "", storageError{err}
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	q := s.q.WithTx(tx)
	if err := lockRecovery(ctx, q, expected.RecoveryID); err != nil {
		return "", err
	}
	email, err := lockEmailIssueAccount(ctx, q, subject, expected.AuthGeneration, purpose)
	if err != nil {
		return "", err
	}
	// Take database time after acquiring the serializing locks. Both admission
	// reads are separate statements and observe a prior issuer's commit.
	now, err := q.EmailIssueTime(ctx)
	if err != nil {
		return "", storageError{err}
	}
	if !now.Valid || now.InfinityModifier != pgtype.Finite {
		return "", storageError{identity.ErrInvalid}
	}
	if err := checkEmailIssueWindow(ctx, q, subject, expected, purpose, now); err != nil {
		return "", err
	}
	id, err := enqueueEmailToken(ctx, tx, q, subject, expected, email, purpose, now, protector, reserve, enqueue)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", storageError{err}
	}
	return id, nil
}

func enqueueEmailToken(ctx context.Context, tx pgx.Tx, q *identitydb.Queries, subject string, expected identity.SessionVersion,
	email identity.Email, purpose emailtoken.Purpose, now pgtype.Timestamptz, protector *emailtoken.Protector,
	reserve EmailTokenSealBudget, enqueue EmailTokenEnqueue,
) (string, error) {
	if err := reserve(ctx, protector.CurrentKeyID()); err != nil {
		return "", storageError{err}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Reservation is conservative: later failures never refund it.
	id, err := persistEmailToken(ctx, q, subject, expected, email, purpose, now, protector)
	if err != nil {
		return "", err
	}
	if err := enqueue(ctx, tx, id); err != nil {
		return "", storageError{err}
	}
	return id, nil
}

func lockEmailIssueAccount(ctx context.Context, q *identitydb.Queries, subject string, generation int64, purpose emailtoken.Purpose) (identity.Email, error) {
	account, err := q.EmailIssueAccount(ctx, subject)
	if err != nil {
		return identity.Email{}, emailIssueReadError(err)
	}
	if !account.Enabled || account.AuthGeneration != generation {
		return identity.Email{}, identity.ErrConflict
	}
	row, err := q.EmailForTokenIssue(ctx, subject)
	if err != nil {
		return identity.Email{}, emailIssueReadError(err)
	}
	if row.EmailVerified != (purpose == emailtoken.Recovery) {
		return identity.Email{}, identity.ErrConflict
	}
	email, err := identity.ParseEmail(row.OriginalEmail)
	if err != nil || email.Key() != row.EmailKey {
		return identity.Email{}, storageError{identity.ErrInvalid}
	}
	return email, nil
}

func emailIssueReadError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrConflict
	}
	return storageError{err}
}

func checkEmailIssueWindow(ctx context.Context, q *identitydb.Queries, subject string, expected identity.SessionVersion, purpose emailtoken.Purpose, now pgtype.Timestamptz) error {
	recent, err := q.EmailRecentIssue(ctx, identitydb.EmailRecentIssueParams{
		SubjectID: subject, Purpose: string(purpose),
		Cutoff: pgtype.Timestamptz{Time: now.Time.Add(-time.Minute), Valid: true},
	})
	if err != nil {
		return storageError{err}
	}
	if recent {
		return identity.ErrConflict
	}
	live, err := q.EmailLiveTokenCount(ctx, identitydb.EmailLiveTokenCountParams{
		SubjectID: subject, Purpose: string(purpose), RecoveryID: expected.RecoveryID, AuthGeneration: expected.AuthGeneration, IssuedAt: now,
	})
	if err != nil {
		return storageError{err}
	}
	if live >= 3 {
		return identity.ErrConflict
	}
	return nil
}

func persistEmailToken(ctx context.Context, q *identitydb.Queries, subject string, expected identity.SessionVersion, email identity.Email,
	purpose emailtoken.Purpose, now pgtype.Timestamptz, protector *emailtoken.Protector,
) (string, error) {
	var rawID [16]byte
	if _, err := rand.Read(rawID[:]); err != nil {
		return "", storageError{err}
	}
	id := hex.EncodeToString(rawID[:])
	token, err := emailtoken.New()
	if err != nil {
		return "", storageError{err}
	}
	digest, err := token.Digest(purpose)
	if err != nil {
		return "", storageError{err}
	}
	scope := emailtoken.Scope{PayloadID: id, SubjectID: subject, RecoveryID: expected.RecoveryID, AuthGeneration: expected.AuthGeneration,
		Recipient: email, Purpose: purpose, IssuedAt: now.Time, SendUntil: now.Time.Add(emailtoken.MaxSendWindow),
		TokenExpiry: now.Time.Add(purpose.Lifetime())}
	payload, err := protector.Seal(now.Time, scope, token)
	if err != nil {
		return "", storageError{err}
	}
	err = q.InsertEmailToken(ctx, identitydb.InsertEmailTokenParams{
		ID: id, SubjectID: subject, Purpose: string(purpose), RecoveryID: expected.RecoveryID, AuthGeneration: expected.AuthGeneration,
		Digest: digest[:], IssuedAt: now, ExpiresAt: pgtype.Timestamptz{Time: scope.TokenExpiry, Valid: true},
	})
	if err != nil {
		return "", storageError{err}
	}
	err = q.InsertEmailPayload(ctx, identitydb.InsertEmailPayloadParams{
		TokenID: id, KeyID: payload.KeyID, Ciphertext: payload.Ciphertext, SendUntil: pgtype.Timestamptz{Time: scope.SendUntil, Valid: true},
	})
	if err != nil {
		return "", storageError{err}
	}
	return id, nil
}

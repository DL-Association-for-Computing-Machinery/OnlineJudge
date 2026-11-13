package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

// RegisterPersonal creates a new unverified account and enqueues its initial
// activation atomically. It generates and discards the random placeholder
// password before starting the transaction, using the shared KDF budget.
// The caller performs shared request admission and hides address availability.
// Existing accounts are never modified. Commit errors may have unknown outcomes.
func (s *Store) RegisterPersonal(ctx context.Context, email identity.Email, recoveryID string, work *password.BoundedHasher,
	protector *emailtoken.Protector, reserve EmailTokenSealBudget, enqueue EmailTokenEnqueue,
) error {
	if err := s.checkPersonal(ctx); err != nil {
		return err
	}
	if !email.Valid() || !identity.ValidRecoveryID(recoveryID) || work.MaxPasswordBytes() < 32 ||
		protector.CurrentKeyID() == "" || reserve == nil || enqueue == nil {
		return identity.ErrInvalid
	}
	var subject [16]byte
	if _, err := rand.Read(subject[:]); err != nil {
		return storageError{err}
	}
	id := hex.EncodeToString(subject[:])
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return storageError{err}
	}
	record, err := work.Create(ctx, secret[:])
	clear(secret[:])
	if err != nil {
		return err
	}
	defer clear(record.Salt)
	defer clear(record.Digest)
	if err := s.checkWrite(id, record); err != nil {
		return err
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
	if err := lockRecovery(ctx, q, recoveryID); err != nil {
		return err
	}
	personal, err := s.initializePersonal(ctx, q, id, email, true, record)
	if err != nil {
		return err
	}
	now, err := q.EmailIssueTime(ctx)
	if err != nil {
		return storageError{err}
	}
	if !now.Valid || now.InfinityModifier != pgtype.Finite {
		return storageError{identity.ErrInvalid}
	}
	expected := identity.SessionVersion{AuthGeneration: personal.AuthGeneration, RecoveryID: recoveryID}
	if _, err := enqueueEmailToken(ctx, tx, q, id, expected, personal.Email, emailtoken.Activation, now, protector, reserve, enqueue); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return storageError{err}
	}
	return nil
}

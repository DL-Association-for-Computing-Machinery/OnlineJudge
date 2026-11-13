package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
)

// LockTrainingAccount checks the personal account requirement under the same
// short READ COMMITTED transaction as a training operation. Use the ordinary
// session's authoritative database. Call after a fresh session read and before
// acquiring resource locks. Recovery, account, and email locks last until
// commit or rollback; on error, roll back. The caller must still enforce
// resource authorization. Do not perform external I/O or write the response
// under these locks. Single-token logout follows the ordinary session's
// accepted-in-flight contract.
func LockTrainingAccount(ctx context.Context, tx pgx.Tx, actor identity.TrainingActor) error {
	if ctx == nil || tx == nil || !actor.Valid() {
		return identity.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	q := identitydb.New(tx)
	if err := lockRecovery(ctx, q, actor.RecoveryID); err != nil {
		return err
	}
	account, err := q.GetStateForShare(ctx, actor.SubjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrConflict
	}
	if err != nil {
		return storageError{err}
	}
	if account.ID != actor.SubjectID || account.CredentialVersion < 1 {
		return storageError{identity.ErrInvalid}
	}
	if !account.Enabled || account.AuthGeneration != actor.AuthGeneration {
		return identity.ErrConflict
	}
	verified, err := q.TrainingEmailForShare(ctx, actor.SubjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrTrainingAccountRequired
	}
	if err != nil {
		return storageError{err}
	}
	if !verified {
		return identity.ErrTrainingAccountRequired
	}
	return ctx.Err()
}

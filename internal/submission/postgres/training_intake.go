package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres/submissiondb"
)

// AcceptTraining atomically rechecks a fresh ordinary account and enqueues
// personal training work using this server-selected immutable material version.
// The caller must authorize the selected version before calling; fixture
// registration is not publication. Use only the ordinary identity database.
func (s *Store) AcceptTraining(ctx context.Context, actor identity.TrainingActor, key, language, source string) (submission.Receipt, bool, error) {
	if s == nil || s.pool == nil || ctx == nil || !actor.Valid() {
		return submission.Receipt{}, false, submission.ErrInvalid
	}
	req := submission.Request{MaterialVersion: s.version, Language: language, Source: source}
	return s.accept(ctx, actor.SubjectID, submission.PersonalTrainingScope, key, req, submission.Digest(req),
		func(ctx context.Context, tx pgx.Tx) error { return identitypg.LockTrainingAccount(ctx, tx, actor) })
}

// AcceptPublishedTraining binds a key to the displayed publication as well as
// the immutable material. The supplied authorization uses the same transaction.
func (s *Store) AcceptPublishedTraining(ctx context.Context, actor identity.TrainingActor, key, problemID, version, source string, allow func(context.Context, pgx.Tx) error) (submission.Receipt, bool, error) {
	if s == nil || s.pool == nil || ctx == nil || !actor.Valid() || allow == nil {
		return submission.Receipt{}, false, submission.ErrInvalid
	}
	req := submission.Request{MaterialVersion: s.version, Language: "gnu++17", Source: source}
	sum := sha256.Sum256([]byte("published-training\x00" + problemID + "\x00" + version + "\x00" + submission.Digest(req)))
	return s.accept(ctx, actor.SubjectID, submission.PersonalTrainingScope, key, req, hex.EncodeToString(sum[:]), func(ctx context.Context, tx pgx.Tx) error {
		if err := identitypg.LockTrainingAccount(ctx, tx, actor); err != nil {
			return err
		}
		return allow(ctx, tx)
	})
}

// ReadTrainingReceipt recovers the original key in the currently authorized
// account's personal scope. An absent receipt cannot exclude an in-flight write.
func ReadTrainingReceipt(ctx context.Context, pool *pgxpool.Pool, actor identity.TrainingActor, key string) (submission.Receipt, error) {
	if ctx == nil || pool == nil || !actor.Valid() || !submission.ValidKey(key) {
		return submission.Receipt{}, submission.ErrInvalid
	}
	return readTraining(ctx, pool, actor, func(ctx context.Context, q *submissiondb.Queries) (submission.Receipt, error) {
		return lookupReceipt(ctx, q, actor.SubjectID, submission.PersonalTrainingScope, key)
	})
}

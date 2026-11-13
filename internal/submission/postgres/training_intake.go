package postgres

import (
	"context"

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
	return s.accept(ctx, actor.SubjectID, submission.PersonalTrainingScope, key,
		submission.Request{MaterialVersion: s.version, Language: language, Source: source},
		func(ctx context.Context, tx pgx.Tx) error { return identitypg.LockTrainingAccount(ctx, tx, actor) })
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

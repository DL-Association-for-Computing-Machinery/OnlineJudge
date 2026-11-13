//go:build sandbox

package sandbox_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func acceptAPlusBTraining(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *postgres.Store, key, source string) (identity.TrainingActor, submission.Receipt) {
	t.Helper()
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("initialize A+B training fixture: %v", err)
		}
	}
	hasher, err := password.New(password.Policy{Current: password.Parameters{
		MemoryKiB: 128, Iterations: 2, Parallelism: 1, SaltBytes: 16, DigestBytes: 32,
	}}, password.Limits{
		MaxMemoryKiB: 128, MaxIterations: 2, MaxParallelism: 1, MaxPasswordBytes: 64, MaxSaltBytes: 16, MaxDigestBytes: 32,
	})
	check(err)
	accounts, err := identitypg.New(pool, hasher)
	check(err)
	epoch, err := accounts.BeginRecovery(ctx)
	check(err)
	check(accounts.FinishRecovery(ctx, epoch))
	email, err := identity.ParseEmail("training-sandbox@example.invalid")
	check(err)
	record, err := hasher.Create([]byte("synthetic sandbox fixture password"))
	check(err)
	p, err := accounts.InitializePersonal(ctx, "00000000000000000000000000000ab1", email, true, record)
	check(err)
	_, err = pool.Exec(ctx, "UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1", p.ID)
	check(err)
	actor := identity.TrainingActor{SubjectID: p.ID, SessionVersion: identity.SessionVersion{
		AuthGeneration: p.AuthGeneration, RecoveryID: epoch,
	}}
	r, created, err := store.AcceptTraining(ctx, actor, key, "gnu++17", source)
	if err != nil || !created || r.State != "queued" {
		t.Fatalf("stable account failed to accept fixed training material: %v", err)
	}
	recovered, err := postgres.ReadTrainingReceipt(ctx, pool, actor, key)
	if err != nil || recovered.ID != r.ID {
		t.Fatalf("cannot recover the original training receipt: %v", err)
	}
	return actor, r
}

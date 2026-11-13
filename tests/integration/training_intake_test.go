//go:build integration

package integration_test

import (
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

type trainingAcceptance struct {
	receipt submission.Receipt
	created bool
	err     error
}

func TestAuthenticatedTrainingIntake(t *testing.T) {
	f := newBacklogFixture(t)
	backlogPolicy(t, f, 4, 1)
	h := identityHasher(t, false)
	accounts, err := identitypg.New(f.a, h)
	must(t, err)
	epoch, err := accounts.BeginRecovery(f.ctx)
	must(t, err)
	must(t, accounts.FinishRecovery(f.ctx, epoch))
	create := func(id int, email string) (identity.Personal, identity.TrainingActor) {
		t.Helper()
		address, err := identity.ParseEmail(email)
		must(t, err)
		person, err := accounts.InitializePersonal(f.ctx, identityID(id), address, true, identityRecord(t, h, "synthetic training intake password"))
		must(t, err)
		return person, identity.TrainingActor{SubjectID: person.ID, SessionVersion: identity.SessionVersion{AuthGeneration: person.AuthGeneration, RecoveryID: epoch}}
	}
	verified := func(subject string) {
		t.Helper()
		_, err := f.a.Exec(f.ctx, "UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1", subject)
		must(t, err)
	}
	person, owner := create(921, "intake-owner@example.invalid")
	_, other := create(922, "intake-other@example.invalid")
	const key = "training-original-key-921"
	put := func(store *postgres.Store, actor identity.TrainingActor, key, source string) trainingAcceptance {
		r, created, err := store.AcceptTraining(f.ctx, actor, key, "gnu++17", source)
		return trainingAcceptance{r, created, err}
	}
	if r := put(f.first, owner, key, f.request.Source); !errors.Is(r.err, identity.ErrTrainingAccountRequired) || r.created || r.receipt.ID != "" {
		t.Fatalf("unverified account produced acceptance: %v", r.err)
	}
	verified(owner.SubjectID)
	verified(other.SubjectID)
	stale := owner
	stale.AuthGeneration++
	if r := put(f.first, stale, key, f.request.Source); !errors.Is(r.err, identity.ErrConflict) || r.created || r.receipt.ID != "" {
		t.Fatalf("stale session produced acceptance: %v", r.err)
	}
	backlogCounts(t, f, 0)

	results := make(chan trainingAcceptance, 2)
	for _, store := range []*postgres.Store{f.first, f.second} {
		go func() { results <- put(store, owner, key, f.request.Source) }()
	}
	first, second := <-results, <-results
	must(t, first.err)
	must(t, second.err)
	if first.created == second.created || first.receipt.ID == "" || first.receipt.ID != second.receipt.ID ||
		first.receipt.MaterialVersion != f.request.MaterialVersion || first.receipt.State != "queued" || first.receipt.Verdict != nil {
		t.Fatal("same-key concurrent calls did not share one queued acceptance")
	}
	backlogCounts(t, f, 1)
	r, err := postgres.ReadTrainingReceipt(f.ctx, f.b, owner, key)
	must(t, err)
	if r.ID != first.receipt.ID {
		t.Fatal("original-key read returned another submission")
	}
	if _, err := postgres.ReadTrainingReceipt(f.ctx, f.b, other, key); !errors.Is(err, submission.ErrNotFound) {
		t.Fatalf("another account found the original key: %v", err)
	}
	if r := put(f.first, owner, key, f.request.Source+" "); !errors.Is(r.err, submission.ErrConflict) || r.created {
		t.Fatalf("mutated replay was accepted: %v", r.err)
	}
	if r := put(f.first, owner, "training-at-quota-921", f.request.Source); !errors.Is(r.err, submission.ErrBacklog) || r.created {
		t.Fatalf("new intent at quota was accepted: %v", r.err)
	}
	backlogCounts(t, f, 1)

	// Reject the task insert after the source insert, then verify neither
	// the source nor its original-key receipt escaped the transaction.
	const rejectedKey = "training-task-failure-922"
	_, err = f.a.Exec(f.ctx, `ALTER TABLE judge_tasks ADD CONSTRAINT fixture_reject_new_task CHECK (state <> 'queued') NOT VALID`)
	must(t, err)
	if r := put(f.first, other, rejectedKey, f.request.Source); r.err == nil || r.created || r.receipt.ID != "" {
		t.Fatalf("task insertion failure returned acceptance: %v", r.err)
	}
	if _, err := postgres.ReadTrainingReceipt(f.ctx, f.a, other, rejectedKey); !errors.Is(err, submission.ErrNotFound) {
		t.Fatalf("failed task left an acceptance: %v", err)
	}
	_, err = f.a.Exec(f.ctx, "ALTER TABLE judge_tasks DROP CONSTRAINT fixture_reject_new_task")
	must(t, err)
	backlogCounts(t, f, 1)

	// Hold the identity change open and observe the real enqueue waiting on
	// that transaction. After commit it must recheck before even replaying.
	tx, err := f.b.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	var pid int32
	must(t, tx.QueryRow(f.ctx, "SELECT pg_backend_pid()").Scan(&pid))
	_, err = tx.Exec(f.ctx, "UPDATE identity_credentials SET enabled=false WHERE id=$1", person.ID)
	must(t, err)
	go func() { results <- put(f.first, owner, key, f.request.Source) }()
	waitIdentityBlocked(t, f.ctx, f.a, pid)
	must(t, tx.Commit(f.ctx))
	if replay := <-results; !errors.Is(replay.err, identity.ErrConflict) || replay.receipt.ID != "" || replay.created {
		t.Fatalf("revoked account recovered a receipt through submit: %v", replay.err)
	}
	if _, err := postgres.ReadTrainingReceipt(f.ctx, f.a, owner, key); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("revoked account recovered a receipt through read: %v", err)
	}
	next, err := accounts.BeginRecovery(f.ctx)
	must(t, err)
	if put := put(f.first, other, rejectedKey, f.request.Source); !errors.Is(put.err, identity.ErrRecovering) || put.created {
		t.Fatalf("intake ran while recovery was closed: %v", put.err)
	}
	must(t, accounts.FinishRecovery(f.ctx, next))
	if put := put(f.first, other, rejectedKey, f.request.Source); !errors.Is(put.err, identity.ErrConflict) || put.created {
		t.Fatalf("intake accepted the former recovery generation: %v", put.err)
	}
	backlogCounts(t, f, 1)
}

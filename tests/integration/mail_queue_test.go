//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
)

func TestMailQueueMigrationAndIssuance(t *testing.T) {
	f := newBacklogFixture(t)
	if _, err := mailqueue.NewEnqueuer(f.ctx, f.a); err == nil {
		t.Fatal("missing River schema was accepted")
	}
	lock, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = lock.Rollback(f.ctx) }()
	_, err = lock.Exec(f.ctx, "SELECT pg_advisory_xact_lock($1)", db.MigrationLockID)
	must(t, err)
	if err := mailqueue.MigrateUp(f.ctx, f.b); !errors.Is(err, mailqueue.ErrMigrationBusy) {
		t.Fatalf("migration did not respect the maintenance lock: %v", err)
	}
	must(t, lock.Rollback(f.ctx))

	// Simulate a maintenance process that stopped after a committed prefix.
	m, err := rivermigrate.New(riverpgxv5.New(f.a), nil)
	must(t, err)
	up, err := m.Migrate(f.ctx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{TargetVersion: 5})
	must(t, err)
	if len(up.Versions) != 5 {
		t.Fatal("test did not start from a partial migration")
	}
	if _, err := mailqueue.NewEnqueuer(f.ctx, f.a); err == nil {
		t.Fatal("partial River schema was accepted")
	}
	must(t, mailqueue.MigrateUp(f.ctx, f.a))
	must(t, mailqueue.MigrateUp(f.ctx, f.b))
	must(t, db.CheckSchema(f.ctx, f.a))
	_, err = f.a.Exec(f.ctx, "DELETE FROM river_migration WHERE line='main' AND version=6")
	must(t, err)
	if _, err := mailqueue.NewEnqueuer(f.ctx, f.b); err == nil {
		t.Fatal("River migration history with a gap was accepted")
	}
	if err := mailqueue.MigrateUp(f.ctx, f.b); err == nil {
		t.Fatal("migrator accepted a gap in history")
	}
	_, err = f.a.Exec(f.ctx, "INSERT INTO river_migration(line,version) VALUES ('main',6)")
	must(t, err)
	must(t, mailqueue.MigrateUp(f.ctx, f.a))
	queue, err := mailqueue.NewEnqueuer(f.ctx, f.a)
	must(t, err)
	h := identityHasher(t, false)
	issuer, err := identitypg.New(f.a, h)
	must(t, err)
	addr, err := identity.ParseEmail("queue@example.invalid")
	must(t, err)
	subject := identityID(1)
	_, err = issuer.InitializePersonal(f.ctx, subject, addr, true, identityRecord(t, h, "synthetic queue issuance password"))
	must(t, err)
	epoch, err := issuer.BeginRecovery(f.ctx)
	must(t, err)
	must(t, issuer.FinishRecovery(f.ctx, epoch))
	version := identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch}
	protector, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	reserve := func(context.Context, *emailtoken.Protector) error { return nil }
	counts := func(want int) {
		t.Helper()
		var jobs, payloads, tokens int
		must(t, f.b.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM river_job),
			(SELECT count(*) FROM identity_email_payloads), (SELECT count(*) FROM identity_email_tokens)`).Scan(&jobs, &payloads, &tokens))
		if jobs != want || payloads != want || tokens != want {
			t.Fatalf("expected %d rows in each table; got %d/%d/%d", want, jobs, payloads, tokens)
		}
	}
	abort := errors.New("fixture abort after successful enqueue")
	_, err = issuer.IssueEmailToken(f.ctx, subject, version, emailtoken.Activation, protector, reserve,
		func(ctx context.Context, tx pgx.Tx, id string) error {
			if err := queue.Enqueue(ctx, tx, id); err != nil {
				return err
			}
			counts(0)
			return abort
		})
	if !errors.Is(err, abort) {
		t.Fatalf("expected rollback after insertion: %v", err)
	}
	counts(0)
	_, err = f.a.Exec(f.ctx, "ALTER TABLE river_job ADD CONSTRAINT fixture_refuses_job CHECK (false)")
	must(t, err)
	if _, err := issuer.IssueEmailToken(f.ctx, subject, version, emailtoken.Activation, protector, reserve, queue.Enqueue); !errors.Is(err, identity.ErrStore) {
		t.Fatalf("expected refusal from queue insertion: %v", err)
	}
	counts(0)
	_, err = f.a.Exec(f.ctx, "ALTER TABLE river_job DROP CONSTRAINT fixture_refuses_job")
	must(t, err)
	id, err := issuer.IssueEmailToken(f.ctx, subject, version, emailtoken.Activation, protector, reserve, queue.Enqueue)
	must(t, err)
	counts(1)
	var correct bool
	must(t, f.b.QueryRow(f.ctx, `SELECT args::jsonb = jsonb_build_object('payload_id', $1::text)
		AND queue = 'account_email' AND kind = 'account_email_delivery' AND max_attempts = 6
		FROM river_job`, id).Scan(&correct))
	if !correct {
		t.Fatal("queue job did not contain the expected ID-only arguments and attempt budget")
	}

	_, err = f.a.Exec(f.ctx, "INSERT INTO river_migration(line,version) VALUES ('main',999)")
	must(t, err)
	if _, err := mailqueue.NewEnqueuer(f.ctx, f.b); err == nil {
		t.Fatal("future River schema was accepted")
	}
	if err := mailqueue.MigrateUp(f.ctx, f.b); err == nil {
		t.Fatal("migration accepted an unknown installed version")
	}
}

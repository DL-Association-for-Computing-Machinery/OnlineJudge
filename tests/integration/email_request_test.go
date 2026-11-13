//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailbudget"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
)

func TestBackgroundEmailRequest(t *testing.T) {
	f := newBacklogFixture(t)
	h := identityHasher(t, false)
	s, err := identitypg.New(f.a, h)
	must(t, err)
	work, err := password.NewBounded(h, password.AdmissionLimits{MaxInFlight: 1})
	must(t, err)
	keys, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	must(t, mailqueue.MigrateUp(f.ctx, f.a))
	queue, err := mailqueue.NewEnqueuer(f.ctx, f.a)
	must(t, err)
	email, err := identity.ParseEmail("intake+test@example.invalid")
	must(t, err)
	epoch, err := s.BeginRecovery(f.ctx)
	must(t, err)
	var dbName string
	must(t, f.a.QueryRow(f.ctx, "SELECT current_schema()").Scan(&dbName))
	name := pgx.Identifier{dbName}.Sanitize()
	_, err = f.a.Exec(f.ctx, "CREATE DATABASE "+name)
	must(t, err)
	t.Cleanup(func() { _, err := f.a.Exec(f.ctx, "DROP DATABASE "+name); must(t, err) })
	cfg := f.a.Config()
	cfg.ConnConfig.Database = dbName
	delete(cfg.ConnConfig.RuntimeParams, "search_path")
	pool, err := pgxpool.NewWithConfig(f.ctx, cfg)
	must(t, err)
	t.Cleanup(pool.Close)
	must(t, mailbudget.Initialize(f.ctx, pool))
	must(t, mailbudget.Provision(f.ctx, pool, keys, 4))
	budget, err := mailbudget.New(pool, keys)
	must(t, err)
	used := func() int {
		t.Helper()
		var seals int
		must(t, pool.QueryRow(f.ctx, "SELECT used FROM public.oj_mail_seal_budget").Scan(&seals))
		return seals
	}
	run := func(p emailtoken.Purpose, generation string, want int) {
		t.Helper()
		must(t, s.ProcessEmailRequest(f.ctx, email, p, generation, work, keys, budget.Reserve, queue.Enqueue))
		var jobs int
		must(t, f.b.QueryRow(f.ctx, "SELECT count(*) FROM river_job").Scan(&jobs))
		seals := used()
		if jobs != want || seals != want {
			t.Fatalf("mail effects: got %d jobs and %d seals, want %d", jobs, seals, want)
		}
	}
	run(emailtoken.Activation, epoch, 0)
	must(t, s.FinishRecovery(f.ctx, epoch))
	run(emailtoken.Activation, identityID(99), 0)
	run(emailtoken.Recovery, epoch, 0)
	run(emailtoken.Activation, epoch, 1)
	run(emailtoken.Activation, epoch, 1)
	run(emailtoken.Recovery, epoch, 1)
	account, err := s.LookupPersonalByEmail(f.ctx, email)
	must(t, err)
	var payload string
	must(t, f.a.QueryRow(f.ctx, "SELECT args->>'payload_id' FROM river_job").Scan(&payload))
	reader, err := identitypg.NewEmailDeliveryReader(f.a, keys)
	must(t, err)
	mail, err := reader.Prepare(f.ctx, payload)
	must(t, err)
	// Move fixture timestamps together to get past the resend interval.
	_, err = f.a.Exec(f.ctx, "UPDATE identity_email_tokens SET issued_at=issued_at-interval '61 seconds', expires_at=expires_at-interval '61 seconds' WHERE id=$1", payload)
	must(t, err)
	run(emailtoken.Activation, epoch, 2)
	var newest string
	must(t, f.a.QueryRow(f.ctx, "SELECT args->>'payload_id' FROM river_job WHERE args->>'payload_id'<>$1", payload).Scan(&newest))
	mail, err = reader.Prepare(f.ctx, newest)
	must(t, err)
	must(t, s.ActivateEmail(f.ctx, mail.Token(), identityRecord(t, h, "synthetic activated account password")))
	active, err := s.PersonalByID(f.ctx, account.ID)
	must(t, err)
	run(emailtoken.Activation, epoch, 2)
	run(emailtoken.Recovery, epoch, 3)
	after, err := s.PersonalByID(f.ctx, account.ID)
	must(t, err)
	if after != active {
		t.Fatal("recovery request changed account state")
	}
	_, err = s.DisableCAS(f.ctx, account.ID, after.Revision)
	must(t, err)
	run(emailtoken.Recovery, epoch, 3)

	// An error with a conflict cause from the enqueue adapter is a storage
	// failure, not a known denial. The processor must preserve that boundary.
	other, err := identity.ParseEmail("intake-failure@example.invalid")
	must(t, err)
	err = s.ProcessEmailRequest(f.ctx, other, emailtoken.Activation, epoch, work, keys, budget.Reserve,
		func(context.Context, pgx.Tx, string) error { return identity.ErrConflict })
	if !errors.Is(err, identity.ErrStore) {
		t.Fatal("storage failure was masked as a definitive business denial")
	}
	if _, err := s.LookupPersonalByEmail(f.ctx, other); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("failed enqueue left a registered account: %v", err)
	}
	var jobs int
	must(t, f.b.QueryRow(f.ctx, "SELECT count(*) FROM river_job").Scan(&jobs))
	if jobs != 3 {
		t.Fatalf("failed enqueue changed the job count: got %d, want 3", jobs)
	}
	if seals := used(); seals != 4 {
		t.Fatalf("failed enqueue did not retain the independent seal charge: got %d, want 4", seals)
	}
}

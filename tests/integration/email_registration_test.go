//go:build integration

package integration_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
)

func TestRegistrationAtomicallyQueuesActivation(t *testing.T) {
	f := newBacklogFixture(t)
	h := identityHasher(t, false)
	a, err := identitypg.New(f.a, h)
	must(t, err)
	b, err := identitypg.New(f.b, h)
	must(t, err)
	epoch, err := a.BeginRecovery(f.ctx)
	must(t, err)
	must(t, mailqueue.MigrateUp(f.ctx, f.a))
	queue, err := mailqueue.NewEnqueuer(f.ctx, f.a)
	must(t, err)
	protector, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	work, err := password.NewBounded(h, password.AdmissionLimits{MaxInFlight: 2})
	must(t, err)
	mailbox, err := identity.ParseEmail("Mailbox+tag@example.invalid")
	must(t, err)
	alternate, err := identity.ParseEmail("mailbox+TAG@EXAMPLE.INVALID")
	must(t, err)
	var reservations atomic.Int64
	reserve := func(context.Context, string) error { reservations.Add(1); return nil }
	counts := func(want int) {
		t.Helper()
		var creds, mailboxes, tokens, payloads, jobs int
		must(t, f.b.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM identity_credentials),
			(SELECT count(*) FROM identity_personal_emails), (SELECT count(*) FROM identity_email_tokens),
			(SELECT count(*) FROM identity_email_payloads), (SELECT count(*) FROM river_job)`).
			Scan(&creds, &mailboxes, &tokens, &payloads, &jobs))
		if creds != want || mailboxes != want || tokens != want || payloads != want || jobs != want {
			t.Fatalf("expected %d visible rows in all registration tables, got %d/%d/%d/%d/%d", want, creds, mailboxes, tokens, payloads, jobs)
		}
	}
	if err := a.RegisterPersonal(f.ctx, mailbox, epoch, work, protector, reserve, queue.Enqueue); !errors.Is(err, identity.ErrRecovering) {
		t.Fatalf("closed recovery accepted registration: %v", err)
	}
	must(t, a.FinishRecovery(f.ctx, epoch))
	if err := a.RegisterPersonal(f.ctx, mailbox, identityID(99), work, protector, reserve, queue.Enqueue); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("stale epoch accepted registration: %v", err)
	}
	counts(0)
	if reservations.Load() != 0 {
		t.Fatal("refused recovery states consumed a seal reservation")
	}
	abort := errors.New("fixture abort after enqueue")
	err = a.RegisterPersonal(f.ctx, mailbox, epoch, work, protector, reserve, func(ctx context.Context, tx pgx.Tx, id string) error {
		if err := queue.Enqueue(ctx, tx, id); err != nil {
			return err
		}
		counts(0)
		// Recovery needs a conflicting row lock. Probe it from the other pool
		// without sleeping or letting its wait outlive this transaction.
		_, err := f.b.Exec(ctx, "SELECT 1 FROM identity_session_recovery WHERE singleton FOR UPDATE NOWAIT")
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
			t.Fatal("recovery row was not locked through enqueue")
		}
		return abort
	})
	if !errors.Is(err, abort) || reservations.Load() != 1 {
		t.Fatalf("expected a rollback without returning the seal reservation: %v", err)
	}
	counts(0)
	start := make(chan struct{})
	errA, errB := make(chan error, 1), make(chan error, 1)
	go func() {
		<-start
		errA <- a.RegisterPersonal(f.ctx, mailbox, epoch, work, protector, reserve, queue.Enqueue)
	}()
	go func() {
		<-start
		errB <- b.RegisterPersonal(f.ctx, alternate, epoch, work, protector, reserve, queue.Enqueue)
	}()
	close(start)
	first, second := <-errA, <-errB
	if !(first == nil && errors.Is(second, identity.ErrConflict) || second == nil && errors.Is(first, identity.ErrConflict)) {
		t.Fatalf("expected one successful registration: %v / %v", first, second)
	}
	counts(1)
	if reservations.Load() != 2 {
		t.Fatal("duplicate registration reserved a seal")
	}
	account, err := b.LookupPersonalByEmail(f.ctx, mailbox)
	must(t, err)
	if !account.Enabled || account.EmailVerified || account.AuthGeneration != 1 || account.CredentialVersion != 1 {
		t.Fatal("registration must create an unverified account")
	}
	wantEmail := mailbox
	if second == nil {
		wantEmail = alternate
	}
	if account.Email != wantEmail {
		t.Fatal("duplicate registration changed the stored address")
	}
	var id string
	must(t, f.b.QueryRow(f.ctx, "SELECT args->>'payload_id' FROM river_job").Scan(&id))
	reader, err := identitypg.NewEmailDeliveryReader(f.b, protector)
	must(t, err)
	delivery, err := reader.Prepare(f.ctx, id)
	must(t, err)
	if s := delivery.Scope(); s.SubjectID != account.ID || s.Purpose != emailtoken.Activation || s.Recipient != account.Email {
		t.Fatal("first activation was not bound to the newly created account")
	}
	must(t, b.ActivateEmail(f.ctx, delivery.Token(), identityRecord(t, h, "synthetic mailbox owner password")))
	activated, err := a.PersonalByID(f.ctx, account.ID)
	must(t, err)
	if !activated.EmailVerified || activated.AuthGeneration != 2 {
		t.Fatal("activation could not complete registration")
	}
}

//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
)

func TestEmailResendRacesOlderPasswordReset(t *testing.T) {
	f := newBacklogFixture(t)
	must(t, mailqueue.MigrateUp(f.ctx, f.a))
	queue, err := mailqueue.NewEnqueuer(f.ctx, f.a)
	must(t, err)
	h := identityHasher(t, false)
	issuer, err := identitypg.New(f.a, h)
	must(t, err)
	consumer, err := identitypg.New(f.b, h)
	must(t, err)
	protector, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	reader, err := identitypg.NewEmailDeliveryReader(f.b, protector)
	must(t, err)
	addr, err := identity.ParseEmail("ResendRace@example.invalid")
	must(t, err)
	subject := identityID(1)
	_, err = issuer.InitializePersonal(f.ctx, subject, addr, true, identityRecord(t, h, "synthetic original password"))
	must(t, err)
	_, err = f.a.Exec(f.ctx, "UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1", subject)
	must(t, err)
	epoch, err := issuer.BeginRecovery(f.ctx)
	must(t, err)
	must(t, issuer.FinishRecovery(f.ctx, epoch))
	version := identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch}
	reserve := func(context.Context, *emailtoken.Protector) error { return nil }
	issue := func(enqueue identitypg.EmailTokenEnqueue) (string, error) {
		return issuer.IssueEmailToken(f.ctx, subject, version, emailtoken.Recovery, protector, reserve, enqueue)
	}
	oldID, err := issue(queue.Enqueue)
	must(t, err)
	old, err := reader.Prepare(f.ctx, oldID)
	must(t, err)
	if old.Scope().TokenExpiry.Sub(old.Scope().IssuedAt) != 30*time.Minute {
		t.Fatal("the recovery link did not receive a 30-minute lifetime")
	}
	// Model a link already in the inbox and advance past the cooldown while
	// preserving its lifetime. Its delivered token remains unchanged.
	_, err = f.a.Exec(f.ctx, `UPDATE identity_email_tokens SET issued_at=issued_at-interval '61 seconds',
		expires_at=expires_at-interval '61 seconds' WHERE id=$1`, oldID)
	must(t, err)
	chosen := identityRecord(t, h, "synthetic mailbox owner replacement")

	type pendingResend struct {
		id    string
		pid   uint32
		token emailtoken.Token
	}
	staged := make(chan pendingResend, 1)
	committed := make(chan error, 1)
	resume := make(chan struct{})
	resendCtx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	go func() {
		_, err := issuer.IssueEmailToken(resendCtx, subject, version, emailtoken.Recovery, protector, reserve, func(ctx context.Context, tx pgx.Tx, id string) error {
			if err := queue.Enqueue(ctx, tx, id); err != nil {
				return err
			}
			// Copy the synthetic link before commit so it can be presented
			// after the real reset has removed its encrypted payload.
			scope := emailtoken.Scope{PayloadID: id, SubjectID: subject, RecoveryID: epoch,
				AuthGeneration: 1, Recipient: addr, Purpose: emailtoken.Recovery}
			var payload emailtoken.Payload
			err := tx.QueryRow(ctx, `SELECT issued_at, expires_at, send_until, key_id, ciphertext
				FROM identity_email_tokens JOIN identity_email_payloads ON id=token_id WHERE id=$1`, id).
				Scan(&scope.IssuedAt, &scope.TokenExpiry, &scope.SendUntil, &payload.KeyID, &payload.Ciphertext)
			if err != nil {
				return err
			}
			token, err := protector.Open(scope.IssuedAt, scope, payload)
			if err != nil {
				return err
			}
			staged <- pendingResend{id: id, pid: tx.Conn().PgConn().PID(), token: token}
			select {
			case <-resume:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		committed <- err
	}()
	var next pendingResend
	select {
	case next = <-staged:
	case err := <-committed:
		t.Fatalf("resend ended before staging its job: %v", err)
	case <-f.ctx.Done():
		t.Fatal("resend did not stage its job")
	}
	reset := make(chan error, 1)
	go func() { reset <- consumer.ResetPasswordByEmail(f.ctx, old.Token(), chosen) }()
	waitBacklogBlocked(t, f, next.pid, f.appB)
	close(resume)
	must(t, <-committed)
	must(t, <-reset)
	account, err := consumer.PersonalByID(f.ctx, subject)
	must(t, err)
	if account.AuthGeneration != 2 || account.CredentialVersion != 2 {
		t.Fatal("the old link did not win one credential update after the resend")
	}
	var history, payloads, jobs int
	must(t, f.b.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM identity_email_tokens),
		(SELECT count(*) FROM identity_email_payloads), (SELECT count(*) FROM river_job)`).Scan(&history, &payloads, &jobs))
	if history != 2 || payloads != 0 || jobs != 2 {
		t.Fatalf("expected two durable issuance records and jobs but no sendable payloads, got %d/%d/%d", history, payloads, jobs)
	}
	if _, err := reader.Prepare(f.ctx, next.id); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("the committed resend remained eligible for delivery: %v", err)
	}
	for _, token := range []emailtoken.Token{old.Token(), next.token} {
		if err := issuer.ResetPasswordByEmail(f.ctx, token, chosen); !errors.Is(err, identity.ErrConflict) {
			t.Fatalf("a link from the previous generation remained eligible: %v", err)
		}
	}
}

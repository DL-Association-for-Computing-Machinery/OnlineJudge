//go:build integration

package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func TestEmailPayloadRetirement(t *testing.T) {
	f := newBacklogFixture(t)
	h := identityHasher(t, false)
	issuer, err := identitypg.New(f.a, h)
	must(t, err)
	consumer, err := identitypg.New(f.b, h)
	must(t, err)
	addr, err := identity.ParseEmail("retirement@example.invalid")
	must(t, err)
	record := identityRecord(t, h, "synthetic retirement fixture password")
	subject := identityID(1)
	_, err = issuer.InitializePersonal(f.ctx, subject, addr, true, record)
	must(t, err)
	epoch, err := issuer.BeginRecovery(f.ctx)
	must(t, err)
	must(t, issuer.FinishRecovery(f.ctx, epoch))
	version := identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch}
	protector, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	_, err = f.a.Exec(f.ctx, "CREATE TABLE fixture_email_jobs(id text PRIMARY KEY, completed boolean NOT NULL DEFAULT false)")
	must(t, err)
	id, err := issuer.IssueEmailToken(f.ctx, subject, version, emailtoken.Activation, protector,
		func(context.Context, *emailtoken.Protector) error { return nil },
		func(ctx context.Context, tx pgx.Tx, payloadID string) error {
			_, err := tx.Exec(ctx, "INSERT INTO fixture_email_jobs(id) VALUES ($1)", payloadID)
			return err
		})
	must(t, err)
	scope := emailtoken.Scope{PayloadID: id, SubjectID: subject, Recipient: addr, RecoveryID: epoch, AuthGeneration: 1, Purpose: emailtoken.Activation}
	var payload emailtoken.Payload
	must(t, f.b.QueryRow(f.ctx, `SELECT t.issued_at,t.expires_at,p.send_until,p.key_id,p.ciphertext
		FROM identity_email_tokens t JOIN identity_email_payloads p ON p.token_id=t.id WHERE t.id=$1`, id).
		Scan(&scope.IssuedAt, &scope.TokenExpiry, &scope.SendUntil, &payload.KeyID, &payload.Ciphertext))
	token, err := protector.Open(scope.IssuedAt, scope, payload)
	must(t, err)
	digest, err := token.Digest(emailtoken.Activation)
	must(t, err)

	assertState := func(wantCompleted bool) {
		t.Helper()
		var completed, hasPayload, sameLedger bool
		must(t, f.b.QueryRow(f.ctx, `SELECT j.completed,
			EXISTS(SELECT 1 FROM identity_email_payloads WHERE token_id=j.id),
			EXISTS(SELECT 1 FROM identity_email_tokens WHERE id=j.id AND digest=$2 AND expires_at=$3)
			FROM fixture_email_jobs j WHERE j.id=$1`, id, digest[:], scope.TokenExpiry).
			Scan(&completed, &hasPayload, &sameLedger))
		if completed != wantCompleted || hasPayload == wantCompleted || !sameLedger {
			t.Fatal("completion and payload removal diverged, or the token ledger changed")
		}
	}
	// A failed DELETE must never pass a raw database error to the queue.
	_, err = f.a.Exec(f.ctx, `ALTER TABLE fixture_email_jobs ADD CONSTRAINT fixture_blocks_payload_delete
		FOREIGN KEY (id) REFERENCES identity_email_payloads(token_id)`)
	must(t, err)
	failedTx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = failedTx.Rollback(f.ctx) }()
	_, err = failedTx.Exec(f.ctx, "UPDATE fixture_email_jobs SET completed=true WHERE id=$1", id)
	must(t, err)
	if err := identitypg.RetireEmailPayload(f.ctx, failedTx, id); err != identity.ErrStore {
		t.Fatal("failed payload removal must return only the sanitized storage error")
	}
	must(t, failedTx.Rollback(f.ctx))
	assertState(false)
	_, err = f.a.Exec(f.ctx, "ALTER TABLE fixture_email_jobs DROP CONSTRAINT fixture_blocks_payload_delete")
	must(t, err)
	for _, commit := range []bool{false, true} {
		func() {
			tx, err := f.a.Begin(f.ctx)
			must(t, err)
			defer func() { _ = tx.Rollback(f.ctx) }()
			_, err = tx.Exec(f.ctx, "UPDATE fixture_email_jobs SET completed=true WHERE id=$1", id)
			must(t, err)
			must(t, identitypg.RetireEmailPayload(f.ctx, tx, id))
			must(t, identitypg.RetireEmailPayload(f.ctx, tx, id))
			assertState(false)
			if commit {
				must(t, tx.Commit(f.ctx))
			}
		}()
		assertState(commit)
	}
	must(t, consumer.ActivateEmail(f.ctx, token, record))
	state, err := consumer.PersonalByID(f.ctx, subject)
	must(t, err)
	if !state.EmailVerified || state.AuthGeneration != 2 {
		t.Fatal("an already delivered link could not activate after ciphertext removal")
	}
}

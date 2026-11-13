//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func TestEmailPasswordRecovery(t *testing.T) {
	f := newBacklogFixture(t)
	h := identityHasher(t, false)
	first, err := identitypg.New(f.a, h)
	must(t, err)
	second, err := identitypg.New(f.b, h)
	must(t, err)
	addr, err := identity.ParseEmail("Recovery@example.invalid")
	must(t, err)
	subject := identityID(1)
	oldPassword := "synthetic original mailbox password"
	old := identityRecord(t, h, oldPassword)
	_, err = first.InitializePersonal(f.ctx, subject, addr, true, old)
	must(t, err)
	epoch, err := first.BeginRecovery(f.ctx)
	must(t, err)
	must(t, first.FinishRecovery(f.ctx, epoch))
	version := identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch}
	protector, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	_, err = f.a.Exec(f.ctx, "CREATE TABLE synthetic_recovery_queue(payload_id text PRIMARY KEY)")
	must(t, err)
	issue := func(purpose emailtoken.Purpose) (string, emailtoken.Token) {
		t.Helper()
		id, err := first.IssueEmailToken(f.ctx, subject, version, purpose, protector,
			func(context.Context, *emailtoken.Protector) error { return nil },
			func(ctx context.Context, tx pgx.Tx, id string) error {
				_, err := tx.Exec(ctx, "INSERT INTO synthetic_recovery_queue VALUES($1)", id)
				return err
			})
		must(t, err)
		scope := emailtoken.Scope{PayloadID: id, SubjectID: subject, RecoveryID: epoch, AuthGeneration: 1, Recipient: addr, Purpose: purpose}
		var payload emailtoken.Payload
		must(t, f.a.QueryRow(f.ctx, `SELECT issued_at, expires_at, send_until, key_id, ciphertext
			FROM identity_email_tokens JOIN identity_email_payloads ON id=token_id WHERE id=$1`, id).
			Scan(&scope.IssuedAt, &scope.TokenExpiry, &scope.SendUntil, &payload.KeyID, &payload.Ciphertext))
		token, err := protector.Open(scope.IssuedAt, scope, payload)
		must(t, err)
		return id, token
	}
	_, activation := issue(emailtoken.Activation)
	passwords := [2]string{"synthetic first replacement password", "synthetic second replacement password"}
	chosen := identityRecord(t, h, passwords[0])
	alternate := identityRecord(t, h, passwords[1])
	reject := func(token emailtoken.Token) {
		t.Helper()
		if err := second.ResetPasswordByEmail(f.ctx, token, chosen); !errors.Is(err, identity.ErrConflict) {
			t.Fatalf("expected recovery refusal, got %v", err)
		}
	}
	reject(activation)
	verify := func(value bool) {
		t.Helper()
		_, err := f.a.Exec(f.ctx, "UPDATE identity_personal_emails SET email_verified=$2 WHERE subject_id=$1", subject, value)
		must(t, err)
	}
	verify(true)
	shift := func(id string, seconds int64) {
		t.Helper()
		_, err := f.a.Exec(f.ctx, `UPDATE identity_email_tokens
			SET issued_at=issued_at+$2*interval '1 second', expires_at=expires_at+$2*interval '1 second' WHERE id=$1`, id, seconds)
		must(t, err)
	}
	expiredID, expired := issue(emailtoken.Recovery)
	shift(expiredID, -1801)
	reject(expired)
	staleID, stale := issue(emailtoken.Recovery)
	next, err := first.BeginRecovery(f.ctx)
	must(t, err)
	must(t, first.FinishRecovery(f.ctx, next))
	reject(stale)
	shift(staleID, -61)
	epoch, version.RecoveryID = next, next
	firstID, one := issue(emailtoken.Recovery)
	verify(false)
	reject(one)
	verify(true)
	if err := second.ActivateEmail(f.ctx, one, chosen); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("activation accepted a recovery credential: %v", err)
	}
	shift(firstID, 3600)
	reject(one)
	shift(firstID, -3661)
	secondID, two := issue(emailtoken.Recovery)

	// An issued request leaves both the existing password and session usable.
	assertBefore := func() {
		t.Helper()
		state, err := first.PersonalByID(f.ctx, subject)
		must(t, err)
		snapshot, err := second.Get(f.ctx, subject)
		must(t, err)
		match, err := h.Verify([]byte(oldPassword), snapshot.Record)
		must(t, err)
		if !state.EmailVerified || state.AuthGeneration != 1 || state.CredentialVersion != 1 || !match.Match {
			t.Fatal("recovery attempt changed the existing account")
		}
		must(t, first.WithSession(f.ctx, subject, version, func(context.Context, identity.SessionState) error { return nil }))
	}
	assertBefore()
	// Simulate accepted delivery and terminal cleanup. The first link must
	// still reach the credential write and the cleanup of its sibling.
	_, err = f.a.Exec(f.ctx, "DELETE FROM synthetic_recovery_queue WHERE payload_id=$1", firstID)
	must(t, err)
	_, err = f.a.Exec(f.ctx, "DELETE FROM identity_email_payloads WHERE token_id=$1", firstID)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `ALTER TABLE synthetic_recovery_queue ADD CONSTRAINT fixture_cleanup_failure
		FOREIGN KEY (payload_id) REFERENCES identity_email_payloads(token_id)`)
	must(t, err)
	if err := second.ResetPasswordByEmail(f.ctx, one, chosen); !errors.Is(err, identity.ErrStore) {
		t.Fatalf("cleanup failure returned %v", err)
	}
	assertBefore()
	_, err = f.a.Exec(f.ctx, "ALTER TABLE synthetic_recovery_queue DROP CONSTRAINT fixture_cleanup_failure")
	must(t, err)

	start := make(chan struct{})
	a, b := make(chan error, 1), make(chan error, 1)
	go func() { <-start; a <- first.ResetPasswordByEmail(f.ctx, one, chosen) }()
	go func() { <-start; b <- second.ResetPasswordByEmail(f.ctx, two, alternate) }()
	close(start)
	firstErr, secondErr := <-a, <-b
	winner := 0
	switch {
	case firstErr == nil && errors.Is(secondErr, identity.ErrConflict):
	case secondErr == nil && errors.Is(firstErr, identity.ErrConflict):
		winner = 1
	default:
		t.Fatalf("expected one successful reset: %v / %v", firstErr, secondErr)
	}
	reject(one)
	reject(two)
	state, err := first.PersonalByID(f.ctx, subject)
	must(t, err)
	if !state.EmailVerified || state.AuthGeneration != 2 || state.CredentialVersion != 2 {
		t.Fatal("expected one credential update and unchanged email verification")
	}
	snapshot, err := second.Get(f.ctx, subject)
	must(t, err)
	for i, candidate := range []string{passwords[0], passwords[1], oldPassword} {
		match, err := h.Verify([]byte(candidate), snapshot.Record)
		must(t, err)
		if match.Match != (i == winner) {
			t.Fatal("password verification did not reflect the winning reset")
		}
	}
	if err := first.WithSession(f.ctx, subject, version, func(context.Context, identity.SessionState) error {
		t.Error("old session reached the guarded callback")
		return nil
	}); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("old session returned %v", err)
	}
	var history, pending bool
	must(t, f.a.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM identity_email_tokens WHERE id=$1),
		EXISTS(SELECT 1 FROM identity_email_payloads WHERE token_id=$1)`, secondID).Scan(&history, &pending))
	if !history || pending {
		t.Fatal("reset did not retain issuance history and clear pending ciphertext")
	}
}

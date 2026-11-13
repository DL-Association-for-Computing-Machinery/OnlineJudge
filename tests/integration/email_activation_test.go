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
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func TestEmailActivation(t *testing.T) {
	f := newBacklogFixture(t)
	h := identityHasher(t, false)
	first, err := identitypg.New(f.a, h)
	must(t, err)
	second, err := identitypg.New(f.b, h)
	must(t, err)
	addr, err := identity.ParseEmail("Activation@example.invalid")
	must(t, err)
	subject := identityID(1)
	old := identityRecord(t, h, "synthetic placeholder password")
	chosen := identityRecord(t, h, "synthetic mailbox owner password")
	alternate := identityRecord(t, h, "synthetic alternate activation password")
	_, err = first.InitializePersonal(f.ctx, subject, addr, true, old)
	must(t, err)
	epoch, err := first.BeginRecovery(f.ctx)
	must(t, err)
	must(t, first.FinishRecovery(f.ctx, epoch))
	version := identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch}
	protector, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	_, err = f.a.Exec(f.ctx, "CREATE TABLE synthetic_activation_queue(payload_id text PRIMARY KEY)")
	must(t, err)
	issue := func() (string, emailtoken.Token) {
		t.Helper()
		id, err := first.IssueEmailToken(f.ctx, subject, version, emailtoken.Activation, protector,
			func(context.Context, string) error { return nil },
			func(ctx context.Context, tx pgx.Tx, id string) error {
				_, err := tx.Exec(ctx, "INSERT INTO synthetic_activation_queue VALUES($1)", id)
				return err
			})
		must(t, err)
		scope := emailtoken.Scope{PayloadID: id, SubjectID: subject, RecoveryID: epoch, AuthGeneration: 1, Recipient: addr, Purpose: emailtoken.Activation}
		var payload emailtoken.Payload
		must(t, f.a.QueryRow(f.ctx, `SELECT issued_at, expires_at, send_until, key_id, ciphertext
			FROM identity_email_tokens JOIN identity_email_payloads ON id=token_id WHERE id=$1`, id).
			Scan(&scope.IssuedAt, &scope.TokenExpiry, &scope.SendUntil, &payload.KeyID, &payload.Ciphertext))
		token, err := protector.Open(scope.IssuedAt, scope, payload)
		must(t, err)
		return id, token
	}
	age := func(id string, by time.Duration) {
		t.Helper()
		_, err := f.a.Exec(f.ctx, `UPDATE identity_email_tokens
			SET issued_at=issued_at-$2*interval '1 second', expires_at=expires_at-$2*interval '1 second' WHERE id=$1`,
			id, int64(by/time.Second))
		must(t, err)
	}
	expiredID, expired := issue()
	age(expiredID, 25*time.Hour)
	if err := first.ActivateEmail(f.ctx, expired, chosen); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expired activation returned %v", err)
	}
	oldEpochID, oldEpochToken := issue()
	next, err := first.BeginRecovery(f.ctx)
	must(t, err)
	must(t, first.FinishRecovery(f.ctx, next))
	if err := second.ActivateEmail(f.ctx, oldEpochToken, chosen); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("stale epoch activation returned %v", err)
	}
	age(oldEpochID, 61*time.Second)
	epoch, version.RecoveryID = next, next

	id, one := issue()
	assertRejected := func(token emailtoken.Token, want error) {
		t.Helper()
		if err := second.ActivateEmail(f.ctx, token, chosen); !errors.Is(err, want) {
			t.Fatalf("activation refusal: got %v, want %v", err, want)
		}
	}
	assertRejected(emailtoken.Token{}, identity.ErrInvalid)
	unissued, err := emailtoken.New()
	must(t, err)
	assertRejected(unissued, identity.ErrConflict)
	wrongDigest, err := unissued.Digest(emailtoken.Recovery)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `INSERT INTO identity_email_tokens
		(id,subject_id,purpose,recovery_id,auth_generation,digest,issued_at,expires_at)
		SELECT $1,$2,'password-recovery',$3,1,$4,t,t+interval '30 minutes' FROM (SELECT clock_timestamp() t) clock`,
		identityID(99), subject, epoch, wrongDigest[:])
	must(t, err)
	assertRejected(unissued, identity.ErrConflict)
	_, err = f.a.Exec(f.ctx, `UPDATE identity_email_tokens
		SET issued_at=issued_at+interval '1 hour', expires_at=expires_at+interval '1 hour' WHERE id=$1`, id)
	must(t, err)
	assertRejected(one, identity.ErrConflict)
	_, err = f.a.Exec(f.ctx, `UPDATE identity_email_tokens
		SET issued_at=issued_at-interval '1 hour', expires_at=expires_at-interval '1 hour' WHERE id=$1`, id)
	must(t, err)
	_, err = f.a.Exec(f.ctx, "UPDATE identity_credentials SET enabled=false WHERE id=$1", subject)
	must(t, err)
	assertRejected(one, identity.ErrConflict)
	_, err = f.a.Exec(f.ctx, "UPDATE identity_credentials SET enabled=true,auth_generation=2 WHERE id=$1", subject)
	must(t, err)
	assertRejected(one, identity.ErrConflict)
	_, err = f.a.Exec(f.ctx, "UPDATE identity_credentials SET auth_generation=1 WHERE id=$1", subject)
	must(t, err)
	age(id, 61*time.Second)
	_, two := issue()
	// Fail a write after the credential and verification updates; neither
	// may survive if the final payload cleanup cannot complete.
	_, err = f.a.Exec(f.ctx, `ALTER TABLE synthetic_activation_queue ADD CONSTRAINT fixture_cleanup_failure
		FOREIGN KEY (payload_id) REFERENCES identity_email_payloads(token_id)`)
	must(t, err)
	if err := second.ActivateEmail(f.ctx, one, chosen); !errors.Is(err, identity.ErrStore) {
		t.Fatalf("cleanup failure returned %v", err)
	}
	before, err := first.PersonalByID(f.ctx, subject)
	must(t, err)
	if before.EmailVerified || before.AuthGeneration != 1 {
		t.Fatal("failed cleanup changed account")
	}
	_, err = f.a.Exec(f.ctx, "ALTER TABLE synthetic_activation_queue DROP CONSTRAINT fixture_cleanup_failure")
	must(t, err)
	start := make(chan struct{})
	type outcome struct {
		index int
		err   error
	}
	result := make(chan outcome, 2)
	tokens := []emailtoken.Token{one, two}
	records := []password.Record{chosen, alternate}
	for i, store := range []*identitypg.Store{first, second} {
		go func() {
			<-start
			result <- outcome{i, store.ActivateEmail(f.ctx, tokens[i], records[i])}
		}()
	}
	close(start)
	winners, winner := 0, 0
	for range 2 {
		r := <-result
		if r.err == nil {
			winners++
			winner = r.index
		} else if !errors.Is(r.err, identity.ErrConflict) {
			t.Fatalf("concurrent activation returned %v", r.err)
		}
	}
	after, err := first.PersonalByID(f.ctx, subject)
	must(t, err)
	if winners != 1 || !after.EmailVerified || after.AuthGeneration != 2 || after.CredentialVersion != 2 {
		t.Fatal("activation did not make exactly one atomic transition")
	}
	snapshot, err := second.Get(f.ctx, subject)
	must(t, err)
	secrets := []string{"synthetic mailbox owner password", "synthetic alternate activation password"}
	newMatch, err := h.Verify([]byte(secrets[winner]), snapshot.Record)
	must(t, err)
	lostMatch, err := h.Verify([]byte(secrets[1-winner]), snapshot.Record)
	must(t, err)
	oldMatch, err := h.Verify([]byte("synthetic placeholder password"), snapshot.Record)
	must(t, err)
	if !newMatch.Match || lostMatch.Match || oldMatch.Match {
		t.Fatal("only the winning password should remain usable")
	}
	if err := first.WithSession(f.ctx, subject, version, func(context.Context, identity.SessionState) error {
		t.Error("old session reached the guarded callback")
		return nil
	}); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("old session returned %v", err)
	}
	for _, token := range []emailtoken.Token{one, two} {
		if err := second.ActivateEmail(f.ctx, token, chosen); !errors.Is(err, identity.ErrConflict) {
			t.Fatalf("replayed activation returned %v", err)
		}
	}
	var pending, history int
	must(t, f.a.QueryRow(f.ctx, `SELECT
		(SELECT count(*) FROM identity_email_payloads p JOIN identity_email_tokens t ON t.id=p.token_id WHERE t.recovery_id=$1),
		(SELECT count(*) FROM identity_email_tokens)`, epoch).Scan(&pending, &history))
	if pending != 0 || history != 5 {
		t.Fatal("activation did not clear pending ciphertext while retaining history")
	}
}

//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func TestEmailTokenIssue(t *testing.T) {
	f := newBacklogFixture(t)
	_, err := f.provider.DownTo(f.ctx, 20)
	must(t, err)
	if db.CheckSchema(f.ctx, f.a) == nil {
		t.Fatal("previous schema accepted")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	h := identityHasher(t, false)
	a, err := identitypg.New(f.a, h)
	must(t, err)
	b, err := identitypg.New(f.b, h)
	must(t, err)
	email, err := identity.ParseEmail("Issue+One@example.invalid")
	must(t, err)
	const subject = "00000000000000000000000000000001"
	_, err = a.InitializePersonal(f.ctx, subject, email, true, identityRecord(t, h, "synthetic token issue"))
	must(t, err)
	epoch, err := a.BeginRecovery(f.ctx)
	must(t, err)
	version := identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch}
	protector, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	_, err = f.a.Exec(f.ctx, "CREATE TABLE synthetic_mail_queue(payload_id text PRIMARY KEY REFERENCES identity_email_payloads(token_id))")
	must(t, err)
	enqueue := func(ctx context.Context, tx pgx.Tx, id string) error {
		_, err := tx.Exec(ctx, "INSERT INTO synthetic_mail_queue(payload_id) VALUES($1)", id)
		return err
	}
	var seals atomic.Int32
	reserve := func(_ context.Context, key string) error {
		if key != "fixture" {
			return errors.New("wrong fixture key")
		}
		seals.Add(1)
		return nil
	}
	check := func(want int) {
		t.Helper()
		var tokens, payloads, jobs int
		must(t, f.a.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM identity_email_tokens),
			(SELECT count(*) FROM identity_email_payloads), (SELECT count(*) FROM synthetic_mail_queue)`).Scan(&tokens, &payloads, &jobs))
		if tokens != want || payloads != want || jobs != want {
			t.Fatalf("token/payload/job counts = %d/%d/%d, want %d", tokens, payloads, jobs, want)
		}
	}
	issue := func(store *identitypg.Store, v identity.SessionVersion, p emailtoken.Purpose, insert identitypg.EmailTokenEnqueue) (string, error) {
		return store.IssueEmailToken(f.ctx, subject, v, p, protector, reserve, insert)
	}
	deny := func(want error, v identity.SessionVersion, p emailtoken.Purpose) {
		t.Helper()
		if id, err := issue(a, v, p, enqueue); id != "" || !errors.Is(err, want) {
			t.Fatalf("expected refusal %v, got %v", want, err)
		}
	}
	deny(identity.ErrRecovering, version, emailtoken.Activation)
	must(t, a.FinishRecovery(f.ctx, epoch))
	deny(identity.ErrConflict, version, emailtoken.Recovery)
	deny(identity.ErrConflict, identity.SessionVersion{AuthGeneration: 2, RecoveryID: epoch}, emailtoken.Activation)
	budgetErr := errors.New("synthetic budget unavailable")
	if id, err := a.IssueEmailToken(f.ctx, subject, version, emailtoken.Activation, protector,
		func(context.Context, string) error { return budgetErr }, enqueue); id != "" || !errors.Is(err, budgetErr) {
		t.Fatal("failed budget did not stop issuance")
	}
	cause := errors.New("synthetic enqueue failure")
	failing := func(ctx context.Context, tx pgx.Tx, id string) error {
		if err := enqueue(ctx, tx, id); err != nil {
			return err
		}
		return cause
	}
	if id, err := issue(a, version, emailtoken.Activation, failing); id != "" || !errors.Is(err, cause) {
		t.Fatal("queue failure did not abort issuance")
	}
	check(0)

	type result struct {
		id  string
		err error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, store := range []*identitypg.Store{a, b} {
		go func() {
			<-start
			id, err := issue(store, version, emailtoken.Activation, enqueue)
			results <- result{id, err}
		}()
	}
	close(start)
	winner := ""
	for range 2 {
		r := <-results
		if r.err == nil && r.id != "" && winner == "" {
			winner = r.id
		} else if r.id != "" || !errors.Is(r.err, identity.ErrConflict) {
			t.Fatalf("unexpected concurrent issue outcome: %v", r.err)
		}
	}
	if winner == "" {
		t.Fatal("no concurrent issuer won")
	}
	check(1)
	if seals.Load() != 2 {
		t.Fatal("cooldown charged an extra reservation or failed enqueue refunded its reservation")
	}
	scope := emailtoken.Scope{PayloadID: winner, SubjectID: subject, RecoveryID: epoch, AuthGeneration: 1,
		Recipient: email, Purpose: emailtoken.Activation}
	var payload emailtoken.Payload
	var digest []byte
	must(t, f.b.QueryRow(f.ctx, `SELECT issued_at, expires_at, send_until, key_id, ciphertext, digest
		FROM identity_email_tokens JOIN identity_email_payloads ON id=token_id WHERE id=$1`, winner).
		Scan(&scope.IssuedAt, &scope.TokenExpiry, &scope.SendUntil, &payload.KeyID, &payload.Ciphertext, &digest))
	token, err := protector.Open(scope.IssuedAt, scope, payload)
	must(t, err)
	wantDigest, err := token.Digest(emailtoken.Activation)
	must(t, err)
	if !bytes.Equal(digest, wantDigest[:]) || scope.TokenExpiry.Sub(scope.IssuedAt) != 24*time.Hour ||
		scope.SendUntil.Sub(scope.IssuedAt) != 10*time.Minute {
		t.Fatal("persisted digest or timestamps did not match the mail payload")
	}
	// Age records in the disposable database to exercise the cooldown and
	// live-link cap without sleeping for several minutes or changing host time.
	age := func(id string, offset time.Duration) {
		t.Helper()
		_, err := f.a.Exec(f.ctx, `UPDATE identity_email_tokens
			SET issued_at=issued_at - $2 * interval '1 second', expires_at=expires_at - $2 * interval '1 second' WHERE id=$1`,
			id, int64(offset/time.Second))
		must(t, err)
	}
	age(winner, 61*time.Second)
	for range 2 {
		id, err := issue(a, version, emailtoken.Activation, enqueue)
		must(t, err)
		age(id, 61*time.Second)
	}
	deny(identity.ErrConflict, version, emailtoken.Activation)
	check(3)
	age(winner, 25*time.Hour)
	_, err = issue(b, version, emailtoken.Activation, enqueue)
	must(t, err)
	check(4)
	_, err = f.a.Exec(f.ctx, "UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1", subject)
	must(t, err)
	deny(identity.ErrConflict, version, emailtoken.Activation)
	_, err = issue(b, version, emailtoken.Recovery, enqueue)
	must(t, err)
	check(5)
	newEpoch, err := a.BeginRecovery(f.ctx)
	must(t, err)
	must(t, a.FinishRecovery(f.ctx, newEpoch))
	deny(identity.ErrConflict, version, emailtoken.Recovery)
	deny(identity.ErrConflict, identity.SessionVersion{AuthGeneration: 1, RecoveryID: newEpoch}, emailtoken.Recovery)
	_, err = f.a.Exec(f.ctx, "UPDATE identity_credentials SET auth_generation=2 WHERE id=$1", subject)
	must(t, err)
	deny(identity.ErrConflict, identity.SessionVersion{AuthGeneration: 2, RecoveryID: newEpoch}, emailtoken.Recovery)
	_, err = f.provider.DownTo(f.ctx, 20)
	if err == nil || !strings.Contains(err.Error(), "cannot discard issued email tokens") {
		t.Fatalf("nonempty downgrade did not refuse: %v", err)
	}
	must(t, db.CheckSchema(f.ctx, f.a))
	check(5)
	testEmailIssuePlans(t, f, subject, epoch)
}

func testEmailIssuePlans(t *testing.T, f backlogFixture, subject, epoch string) {
	t.Helper()
	_, err := f.a.Exec(f.ctx, `INSERT INTO identity_email_tokens
		(id,subject_id,purpose,recovery_id,auth_generation,digest,issued_at,expires_at)
		SELECT lpad(to_hex(n),32,'0'),$1,'activation',$2,1,sha256(convert_to(n::text,'UTF8')),
			issued,issued+interval '24 hours'
		FROM generate_series(1000,9191) n
		CROSS JOIN (SELECT clock_timestamp()-interval '48 hours' AS issued) timepoint`, subject, epoch)
	must(t, err)
	_, err = f.a.Exec(f.ctx, "ANALYZE identity_email_tokens")
	must(t, err)
	for _, q := range []struct{ sql, index string }{
		{`EXPLAIN (ANALYZE, BUFFERS) SELECT EXISTS (SELECT 1 FROM identity_email_tokens
		  WHERE subject_id=$1 AND purpose='activation'
		  AND issued_at > statement_timestamp()-interval '60 seconds')`, "identity_email_tokens_recent"},
		{`EXPLAIN (ANALYZE, BUFFERS) SELECT count(*) FROM (SELECT 1 FROM identity_email_tokens
		  WHERE subject_id=$1 AND purpose='activation' AND recovery_id=$2 AND auth_generation=1
		  AND expires_at > statement_timestamp() LIMIT 3) live`, "identity_email_tokens_live"},
	} {
		args := []any{subject}
		if q.index == "identity_email_tokens_live" {
			args = append(args, epoch)
		}
		rows, err := f.a.Query(f.ctx, q.sql, args...)
		must(t, err)
		var plan strings.Builder
		for rows.Next() {
			var line string
			must(t, rows.Scan(&line))
			plan.WriteString(line + "\n")
		}
		must(t, rows.Err())
		rows.Close()
		if !strings.Contains(plan.String(), q.index) {
			t.Fatalf("expected bounded index %s in plan: %s", q.index, plan.String())
		}
		t.Log(plan.String())
	}
	var size int64
	must(t, f.a.QueryRow(f.ctx, "SELECT pg_indexes_size('identity_email_tokens')").Scan(&size))
	t.Logf("Token indexes on synthetic history: %d bytes", size)
}

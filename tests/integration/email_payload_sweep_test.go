//go:build integration

package integration_test

import (
	"os"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func TestSweepExpiredEmailPayloads(t *testing.T) {
	f := newBacklogFixture(t)
	h := identityHasher(t, false)
	issuer, err := identitypg.New(f.a, h)
	must(t, err)
	addr, err := identity.ParseEmail("expiry@example.invalid")
	must(t, err)
	subject := identityID(1)
	_, err = issuer.InitializePersonal(f.ctx, subject, addr, true, identityRecord(t, h, "synthetic expiry fixture password"))
	must(t, err)
	epoch, err := issuer.BeginRecovery(f.ctx)
	must(t, err)
	must(t, issuer.FinishRecovery(f.ctx, epoch))
	_, err = f.a.Exec(f.ctx, `INSERT INTO identity_email_tokens
		(id,subject_id,purpose,recovery_id,auth_generation,digest,issued_at,expires_at)
		SELECT lpad(to_hex(n),32,'0'),$1,'activation',$2,1,sha256(int4send(n)),stamp,stamp+interval '24 hours'
		FROM generate_series(1,8192) n CROSS JOIN (SELECT statement_timestamp()-interval '15 minutes' AS stamp) times`, subject, epoch)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `INSERT INTO identity_email_payloads(token_id,key_id,ciphertext,send_until)
		SELECT id,'fixture',decode(repeat('00',60),'hex'),statement_timestamp() +
			CASE WHEN id IN ($1,$2) THEN -interval '1 second' ELSE interval '1 hour' END
		FROM identity_email_tokens`, identityID(1), identityID(3))
	must(t, err)
	_, err = f.a.Exec(f.ctx, "ANALYZE identity_email_payloads")
	must(t, err)
	checkEmailSweepPlan(t, f)

	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, "SELECT token_id FROM identity_email_payloads WHERE token_id=$1 FOR UPDATE", identityID(1))
	must(t, err)
	first, err := identitypg.SweepExpiredEmailPayloads(f.ctx, f.b, "", 2)
	must(t, err)
	if first.NextCursor != identityID(2) || first.Removed != 0 {
		t.Fatal("first page did not skip the locked expired payload and retain the active one")
	}
	second, err := identitypg.SweepExpiredEmailPayloads(f.ctx, f.b, first.NextCursor, 2)
	must(t, err)
	if second.NextCursor != identityID(4) || second.Removed != 1 {
		t.Fatal("next page did not remove only the eligible payload")
	}
	must(t, tx.Rollback(f.ctx))
	again, err := identitypg.SweepExpiredEmailPayloads(f.ctx, f.b, "", 2)
	must(t, err)
	if again.NextCursor != identityID(2) || again.Removed != 1 {
		t.Fatal("next pass did not revisit the previously locked row")
	}
	last, err := identitypg.SweepExpiredEmailPayloads(f.ctx, f.b, identityID(8191), 2)
	must(t, err)
	if last.NextCursor != "" || last.Removed != 0 {
		t.Fatal("short final page did not terminate the pass")
	}
	var ledger, payloads int
	must(t, f.a.QueryRow(f.ctx, `SELECT
		(SELECT count(*) FROM identity_email_tokens), (SELECT count(*) FROM identity_email_payloads)`).
		Scan(&ledger, &payloads))
	if ledger != 8192 || payloads != 8190 {
		t.Fatalf("ledger/payload counts = %d/%d", ledger, payloads)
	}
}

func checkEmailSweepPlan(t *testing.T, f backlogFixture) {
	t.Helper()
	source, err := os.ReadFile("../../db/queries/identity/email_payload_retirement.sql")
	must(t, err)
	_, query, found := strings.Cut(string(source), "-- name: SweepExpiredEmailPayloads :one\n")
	if !found {
		t.Fatal("sweep SQL not found")
	}
	query = strings.NewReplacer("sqlc.arg(after_id)", "$1", "sqlc.arg(batch_size)", "$2").Replace(query)
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	rows, err := tx.Query(f.ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query, "", 512)
	must(t, err)
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		must(t, rows.Scan(&line))
		plan.WriteString(line + "\n")
	}
	must(t, rows.Err())
	if !strings.Contains(plan.String(), "identity_email_payloads_pkey") ||
		strings.Contains(plan.String(), "Seq Scan on identity_email_payloads") {
		t.Fatalf("sparse sweep should use bounded primary key reads:\n%s", plan.String())
	}
	t.Log(plan.String())
}

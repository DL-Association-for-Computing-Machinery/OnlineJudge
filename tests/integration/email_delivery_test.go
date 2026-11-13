//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func TestEmailDeliveryPrepare(t *testing.T) {
	f := newBacklogFixture(t)
	h := identityHasher(t, false)
	issuer, err := identitypg.New(f.a, h)
	must(t, err)
	addr, err := identity.ParseEmail("Mail+tag@example.invalid")
	must(t, err)
	record := identityRecord(t, h, "synthetic email delivery password")
	subject := identityID(1)
	_, err = issuer.InitializePersonal(f.ctx, subject, addr, true, record)
	must(t, err)
	epoch, err := issuer.BeginRecovery(f.ctx)
	must(t, err)
	must(t, issuer.FinishRecovery(f.ctx, epoch))
	version := identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch}
	keys := map[string][32]byte{"fixture": {1}}
	protector, err := emailtoken.NewProtector("fixture", keys)
	must(t, err)
	reader, err := identitypg.NewEmailDeliveryReader(f.b, protector)
	must(t, err)
	_, err = f.a.Exec(f.ctx, "CREATE TABLE synthetic_delivery_queue(payload_id text PRIMARY KEY)")
	must(t, err)
	issue := func(purpose emailtoken.Purpose) string {
		t.Helper()
		id, err := issuer.IssueEmailToken(f.ctx, subject, version, purpose, protector,
			func(context.Context, string) error { return nil },
			func(ctx context.Context, tx pgx.Tx, id string) error {
				_, err := tx.Exec(ctx, "INSERT INTO synthetic_delivery_queue VALUES($1)", id)
				return err
			})
		must(t, err)
		return id
	}
	activationID := issue(emailtoken.Activation)
	checkEmailDeliveryPlan(t, f, activationID)
	prepared, err := reader.Prepare(f.ctx, activationID)
	must(t, err)
	scope := prepared.Scope()
	if scope.PayloadID != activationID || scope.Recipient != addr || scope.Purpose != emailtoken.Activation ||
		scope.SendUntil.Sub(scope.IssuedAt) != emailtoken.MaxSendWindow {
		t.Fatal("prepared mail did not match the issued destination and deadlines")
	}
	digest, err := prepared.Token().Digest(scope.Purpose)
	must(t, err)
	var matches bool
	must(t, f.a.QueryRow(f.ctx, "SELECT digest=$2 FROM identity_email_tokens WHERE id=$1", activationID, digest[:]).Scan(&matches))
	encoded, err := json.Marshal(prepared)
	must(t, err)
	if !matches || fmt.Sprintf("%#v", prepared) != "[redacted email delivery]" || string(encoded) != "{}" {
		t.Fatal("mail result did not match the ledger or was not redacted")
	}
	reject := func(t *testing.T, r *identitypg.EmailDeliveryReader, id string, want error) {
		t.Helper()
		got, err := r.Prepare(f.ctx, id)
		if !errors.Is(err, want) || got.Token().LinkValue() != "" || got.Scope().Recipient.Valid() {
			t.Fatalf("expected refusal without sensitive data, got %v", err)
		}
	}
	reject(t, reader, "", identity.ErrInvalid)
	reject(t, reader, identityID(99), identity.ErrConflict)
	other, err := emailtoken.NewProtector("other", map[string][32]byte{"other": {2}})
	must(t, err)
	wrongReader, err := identitypg.NewEmailDeliveryReader(f.b, other)
	must(t, err)
	reject(t, wrongReader, activationID, identity.ErrStore)

	// Each synthetic fault is applied and undone independently while the same
	// activation is eligible. The sender uses a different pool from the writes.
	for _, tc := range []struct {
		name, apply, undo string
		key               string
		want              error
	}{
		{"disabled", "UPDATE identity_credentials SET enabled=false WHERE id=$1", "UPDATE identity_credentials SET enabled=true WHERE id=$1", subject, identity.ErrConflict},
		{"old account generation", "UPDATE identity_credentials SET auth_generation=2 WHERE id=$1", "UPDATE identity_credentials SET auth_generation=1 WHERE id=$1", subject, identity.ErrConflict},
		{"verified activation", "UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1", "UPDATE identity_personal_emails SET email_verified=false WHERE subject_id=$1", subject, identity.ErrConflict},
		{"future issuance", "UPDATE identity_email_tokens SET issued_at=issued_at+interval '1 hour',expires_at=expires_at+interval '1 hour' WHERE id=$1", "UPDATE identity_email_tokens SET issued_at=issued_at-interval '1 hour',expires_at=expires_at-interval '1 hour' WHERE id=$1", activationID, identity.ErrConflict},
		{"expired link", "UPDATE identity_email_tokens SET issued_at=issued_at-interval '25 hours',expires_at=expires_at-interval '25 hours' WHERE id=$1", "UPDATE identity_email_tokens SET issued_at=issued_at+interval '25 hours',expires_at=expires_at+interval '25 hours' WHERE id=$1", activationID, identity.ErrConflict},
		{"damaged ciphertext", "UPDATE identity_email_payloads SET ciphertext=set_byte(ciphertext,0,get_byte(ciphertext,0)#1) WHERE token_id=$1", "UPDATE identity_email_payloads SET ciphertext=set_byte(ciphertext,0,get_byte(ciphertext,0)#1) WHERE token_id=$1", activationID, identity.ErrStore},
		{"changed address display", "UPDATE identity_personal_emails SET original_email=lower(original_email) WHERE subject_id=$1", "", subject, identity.ErrStore},
		{"digest mismatch", "UPDATE identity_email_tokens SET digest=sha256(digest) WHERE id=$1", "", activationID, identity.ErrStore},
		{"closed send window", "UPDATE identity_email_payloads SET send_until=clock_timestamp()-interval '1 second' WHERE token_id=$1", "", activationID, identity.ErrConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.a.Exec(f.ctx, tc.apply, tc.key)
			must(t, err)
			t.Cleanup(func() {
				var err error
				switch tc.name {
				case "changed address display":
					_, err = f.a.Exec(f.ctx, "UPDATE identity_personal_emails SET original_email=$2 WHERE subject_id=$1", subject, addr.Original())
				case "digest mismatch":
					_, err = f.a.Exec(f.ctx, "UPDATE identity_email_tokens SET digest=$2 WHERE id=$1", activationID, digest[:])
				case "closed send window":
					_, err = f.a.Exec(f.ctx, "UPDATE identity_email_payloads SET send_until=$2 WHERE token_id=$1", activationID, scope.SendUntil)
				default:
					_, err = f.a.Exec(f.ctx, tc.undo, tc.key)
				}
				must(t, err)
			})
			reject(t, reader, activationID, tc.want)
		})
	}
	must(t, issuer.ActivateEmail(f.ctx, prepared.Token(), record))
	reject(t, reader, activationID, identity.ErrConflict)
	version.AuthGeneration++
	recoveryID := issue(emailtoken.Recovery)
	recovery, err := reader.Prepare(f.ctx, recoveryID)
	must(t, err)
	if recovery.Scope().Purpose != emailtoken.Recovery || recovery.Scope().Recipient != addr {
		t.Fatal("verified account did not produce recovery mail")
	}
	_, err = f.a.Exec(f.ctx, "UPDATE identity_personal_emails SET email_verified=false WHERE subject_id=$1", subject)
	must(t, err)
	reject(t, reader, recoveryID, identity.ErrConflict)
	_, err = f.a.Exec(f.ctx, "UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1", subject)
	must(t, err)
	next, err := issuer.BeginRecovery(f.ctx)
	must(t, err)
	reject(t, reader, recoveryID, identity.ErrConflict)
	must(t, issuer.FinishRecovery(f.ctx, next))
	reject(t, reader, recoveryID, identity.ErrConflict)
}

func checkEmailDeliveryPlan(t *testing.T, f backlogFixture, id string) {
	t.Helper()
	// Explain the same source query sqlc binds for the reader.
	source, err := os.ReadFile("../../db/queries/identity/email_delivery.sql")
	must(t, err)
	_, query, found := strings.Cut(string(source), "\n")
	if !found || strings.Count(query, "@payload_id") != 1 {
		t.Fatal("expected the delivery query to take only a payload ID")
	}
	rows, err := f.a.Query(f.ctx, "EXPLAIN (ANALYZE, BUFFERS) "+strings.Replace(query, "@payload_id", "$1", 1), id)
	must(t, err)
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		must(t, rows.Scan(&line))
		plan.WriteString(line + "\n")
	}
	must(t, rows.Err())
	for _, index := range []string{"identity_email_payloads_pkey", "identity_email_tokens_pkey"} {
		if !strings.Contains(plan.String(), index) {
			t.Fatalf("expected delivery lookup to use %s:\n%s", index, plan.String())
		}
	}
	t.Log(plan.String())
}

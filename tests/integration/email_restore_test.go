//go:build integration && linux

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

func TestArchivedEmailAuthorityFencedAfterRestore(t *testing.T) {
	f := newBacklogFixture(t)
	hasher := identityHasher(t, false)
	source, err := identitypg.New(f.a, hasher)
	must(t, err)
	key, err := emailtoken.NewProtector("restore-fixture", map[string][32]byte{"restore-fixture": {1}})
	must(t, err)
	sourceReader, err := identitypg.NewEmailDeliveryReader(f.a, key)
	must(t, err)
	epoch, err := source.BeginRecovery(f.ctx)
	must(t, err)
	must(t, source.FinishRecovery(f.ctx, epoch))
	oldVersion := identity.SessionVersion{RecoveryID: epoch, AuthGeneration: 1}
	record := identityRecord(t, hasher, "synthetic initial mailbox password")
	replacement := identityRecord(t, hasher, "synthetic replacement mailbox password")
	_, err = f.a.Exec(f.ctx, "CREATE TABLE fixture_restore_mail (id text PRIMARY KEY)")
	must(t, err)
	enqueue := func(ctx context.Context, tx pgx.Tx, id string) error {
		_, err := tx.Exec(ctx, "INSERT INTO fixture_restore_mail VALUES ($1)", id)
		return err
	}
	reserve := func(context.Context, string) error { return nil }
	type fixture struct {
		subject, id string
		purpose     emailtoken.Purpose
		token       emailtoken.Token
		account     identity.Snapshot
	}
	var archived []fixture
	for i, purpose := range []emailtoken.Purpose{emailtoken.Activation, emailtoken.Recovery} {
		subject := identityID(i + 1)
		addr, err := identity.ParseEmail(string(purpose) + "@example.invalid")
		must(t, err)
		_, err = source.InitializePersonal(f.ctx, subject, addr, true, record)
		must(t, err)
		if purpose == emailtoken.Recovery {
			_, err = f.a.Exec(f.ctx, "UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1", subject)
			must(t, err)
		}
		id, err := source.IssueEmailToken(f.ctx, subject, oldVersion, purpose, key, reserve, enqueue)
		must(t, err)
		delivery, err := sourceReader.Prepare(f.ctx, id)
		must(t, err)
		account, err := source.Get(f.ctx, subject)
		must(t, err)
		archived = append(archived, fixture{subject: subject, id: id, purpose: purpose, token: delivery.Token(), account: account})
	}

	archive, schema := dumpSubmissionFixture(t, f, t.TempDir())
	consume := func(store *identitypg.Store, mail fixture) error {
		if mail.purpose == emailtoken.Activation {
			return store.ActivateEmail(f.ctx, mail.token, replacement)
		}
		return store.ResetPasswordByEmail(f.ctx, mail.token, replacement)
	}
	for _, mail := range archived {
		must(t, consume(source, mail))
		if err := consume(source, mail); !errors.Is(err, identity.ErrConflict) {
			t.Fatalf("source did not reject reuse after consumption: %v", err)
		}
	}

	target := newSubmissionRestoreTarget(t, f.ctx, schema, archive)
	store, err := identitypg.New(target, hasher)
	must(t, err)
	reader, err := identitypg.NewEmailDeliveryReader(target, key)
	must(t, err)
	for _, mail := range archived {
		delivery, err := reader.Prepare(f.ctx, mail.id)
		must(t, err)
		if delivery.Token() != mail.token {
			t.Fatal("restore did not contain the original eligible payload")
		}
	}
	next, err := store.BeginRecovery(f.ctx)
	must(t, err)
	if next == epoch {
		t.Fatal("recovery reused the archived authority generation")
	}
	assertFenced := func(t *testing.T, wantConsumeError error) {
		t.Helper()
		for _, mail := range archived {
			if got, err := reader.Prepare(f.ctx, mail.id); !errors.Is(err, identity.ErrConflict) || got.Token().LinkValue() != "" {
				t.Fatalf("old payload could still be prepared: %v", err)
			}
			if err := consume(store, mail); !errors.Is(err, wantConsumeError) {
				t.Fatalf("old link did not fail as expected: %v", err)
			}
			got, err := store.PersonalByID(f.ctx, mail.subject)
			must(t, err)
			if got.EmailVerified != (mail.purpose == emailtoken.Recovery) {
				t.Fatal("rejected token changed the restored email verification")
			}
			sameIdentity(t, f.ctx, store, mail.account)
			var preserved bool
			must(t, target.QueryRow(f.ctx, `SELECT EXISTS (SELECT 1 FROM identity_email_payloads p
				JOIN identity_email_tokens t ON t.id=p.token_id
				WHERE p.token_id=$1 AND p.send_until>clock_timestamp() AND t.expires_at>clock_timestamp())`, mail.id).Scan(&preserved))
			if !preserved {
				t.Fatal("old payload was removed or expired before fencing was verified")
			}
		}
	}
	t.Run("closed", func(t *testing.T) { assertFenced(t, identity.ErrRecovering) })
	must(t, store.FinishRecovery(f.ctx, next))
	t.Run("reopened", func(t *testing.T) { assertFenced(t, identity.ErrConflict) })

	// A new account can issue and consume with the new generation. Avoid
	// changing the archived same-purpose resend clock to make this succeed.
	subject := identityID(3)
	addr, err := identity.ParseEmail("new@example.invalid")
	must(t, err)
	_, err = store.InitializePersonal(f.ctx, subject, addr, true, record)
	must(t, err)
	current := identity.SessionVersion{RecoveryID: next, AuthGeneration: 1}
	id, err := store.IssueEmailToken(f.ctx, subject, current, emailtoken.Activation, key, reserve, enqueue)
	must(t, err)
	delivery, err := reader.Prepare(f.ctx, id)
	must(t, err)
	must(t, store.ActivateEmail(f.ctx, delivery.Token(), replacement))
}

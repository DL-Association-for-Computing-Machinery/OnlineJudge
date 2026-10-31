//go:build integration

package integration_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func testPersonalEmails(t *testing.T, ctx context.Context, a, b *pgxpool.Pool) {
	h := identityHasher(t, false)
	first, err := identitypg.New(a, h)
	must(t, err)
	second, err := identitypg.New(b, h)
	must(t, err)
	r := identityRecord(t, h, "synthetic personal password")
	email := personalEmail(t, "User.Name+Tag@Example.invalid")
	t.Run("round trip preserves original and separates verification", func(t *testing.T) {
		row, err := first.InitializePersonal(ctx, identityID(1810), email, true, r)
		must(t, err)
		if row.Email != email || row.EmailVerified || !row.Enabled || row.Revision != (identity.Revision{CredentialVersion: 1, AuthGeneration: 1}) {
			t.Fatal("wrong initial personal state")
		}
		byID, err := second.PersonalByID(ctx, row.ID)
		must(t, err)
		byEmail, err := second.LookupPersonalByEmail(ctx, personalEmail(t, "user.name+tag@EXAMPLE.INVALID"))
		must(t, err)
		if byID != row || byEmail != row {
			t.Fatal("case-insensitive lookup lost original metadata")
		}
		stored, err := second.Get(ctx, row.ID)
		must(t, err)
		sameIdentity(t, ctx, first, stored)
		result, err := h.Verify([]byte("synthetic personal password"), stored.Record)
		must(t, err)
		if !result.Match {
			t.Fatal("personal initialization did not store supplied credentials")
		}
		_, err = second.DisableCAS(ctx, row.ID, row.Revision)
		must(t, err)
		// Only the disposable fixture sets verification. No public activation API
		// exists yet; changing this fact must not revive a disabled subject.
		_, err = a.Exec(ctx, `UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1`, row.ID)
		must(t, err)
		byEmail, err = first.LookupPersonalByEmail(ctx, email)
		must(t, err)
		if byEmail.Enabled || !byEmail.EmailVerified || byEmail.AuthGeneration != 2 {
			t.Fatal("email verification and enabled state were conflated")
		}
	})
	t.Run("conflicts preserve subjects and do not attach generic accounts", func(t *testing.T) {
		before, err := first.Get(ctx, identityID(1810))
		must(t, err)
		for _, candidate := range []struct{ id, raw string }{
			{identityID(1810), "different@example.invalid"},
			{identityID(1811), "USER.NAME+TAG@example.invalid"},
		} {
			if _, err := second.InitializePersonal(ctx, candidate.id, personalEmail(t, candidate.raw), true, r); err != identity.ErrConflict {
				t.Fatal("duplicate ID or email overwrote an existing identity")
			}
		}
		sameIdentity(t, ctx, second, before)
		if _, err := second.Get(ctx, identityID(1811)); err != identity.ErrNotFound {
			t.Fatal("email conflict left an orphan subject")
		}
		generic, err := first.Initialize(ctx, identityID(1812), false, r)
		must(t, err)
		if _, err := second.InitializePersonal(ctx, generic.ID, personalEmail(t, "generic@example.invalid"), true, r); err != identity.ErrConflict {
			t.Fatal("generic subject was rebound to an email")
		}
		sameIdentity(t, ctx, first, generic)
		if _, err := second.PersonalByID(ctx, generic.ID); err != identity.ErrNotFound {
			t.Fatal("generic subject acquired an email")
		}
	})
	t.Run("dot and plus variants remain independent", func(t *testing.T) {
		for i, raw := range []string{"ab@example.invalid", "a.b@example.invalid", "ab+tag@example.invalid"} {
			row, err := first.InitializePersonal(ctx, identityID(1820+i), personalEmail(t, raw), false, r)
			must(t, err)
			got, err := second.LookupPersonalByEmail(ctx, personalEmail(t, raw))
			must(t, err)
			if got != row || got.EmailVerified || got.Enabled {
				t.Fatal("independent alias or initial disabled state lost")
			}
		}
	})
	t.Run("concurrent case variants have one owner and no loser credentials", func(t *testing.T) {
		testPersonalCompetition(t, ctx, first, second, r, false)
	})
	t.Run("concurrent subject initialization has one mapping", func(t *testing.T) {
		testPersonalCompetition(t, ctx, first, second, r, true)
	})
	t.Run("validation and missing reads", func(t *testing.T) {
		if _, err := first.InitializePersonal(ctx, identityID(1850), identity.Email{}, true, r); err != identity.ErrInvalid {
			t.Fatal("zero email initialized")
		}
		if _, err := first.InitializePersonal(ctx, "email@example.invalid", email, true, r); err != identity.ErrInvalid {
			t.Fatal("email used as opaque subject ID")
		}
		legacy := identityRecord(t, identityHasher(t, true), "synthetic legacy")
		if _, err := first.InitializePersonal(ctx, identityID(1850), email, true, legacy); err != password.ErrRecord {
			t.Fatal("personal initializer accepted legacy record")
		}
		if _, err := first.LookupPersonalByEmail(ctx, identity.Email{}); err != identity.ErrInvalid {
			t.Fatal("zero lookup accepted")
		}
		if _, err := first.PersonalByID(ctx, "nickname"); err != identity.ErrInvalid {
			t.Fatal("nickname used as identity")
		}
		if _, err := first.LookupPersonalByEmail(ctx, personalEmail(t, "missing@example.invalid")); err != identity.ErrNotFound {
			t.Fatal("missing email was not explicit")
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := first.InitializePersonal(cancelled, identityID(1850), email, true, r); !errors.Is(err, context.Canceled) {
			t.Fatal("pre-canceled initializer started storage")
		}
		if _, err := second.Get(ctx, identityID(1850)); err != identity.ErrNotFound {
			t.Fatal("invalid or canceled operation left credentials")
		}
	})
	t.Run("failed second insert rolls back credentials and hides details", func(t *testing.T) {
		_, err := a.Exec(ctx, `CREATE FUNCTION reject_personal_insert() RETURNS trigger LANGUAGE plpgsql AS $$
            BEGIN RAISE EXCEPTION 'synthetic private email and credential details' USING ERRCODE='P0001'; END $$;
            CREATE TRIGGER reject_personal AFTER INSERT ON identity_personal_emails FOR EACH ROW EXECUTE FUNCTION reject_personal_insert()`)
		must(t, err)
		defer func() {
			_, err := a.Exec(ctx, `DROP TRIGGER reject_personal ON identity_personal_emails; DROP FUNCTION reject_personal_insert()`)
			must(t, err)
		}()
		got, err := first.InitializePersonal(ctx, identityID(1851), personalEmail(t, "failure@example.invalid"), true, r)
		if !errors.Is(err, identity.ErrStore) || err.Error() != identity.ErrStore.Error() || got != (identity.Personal{}) {
			t.Fatal("failed transaction leaked result or private error details")
		}
		if _, err := second.Get(ctx, identityID(1851)); err != identity.ErrNotFound {
			t.Fatal("failed second insert left committed credentials")
		}
		if _, err := second.LookupPersonalByEmail(ctx, personalEmail(t, "failure@example.invalid")); err != identity.ErrNotFound {
			t.Fatal("failed second insert left a mapping")
		}
	})
	t.Run("database constraints and fail-closed metadata", func(t *testing.T) {
		for _, query := range []string{
			`UPDATE identity_personal_emails SET email_key=upper(email_key) WHERE subject_id=$1`,
			`UPDATE identity_personal_emails SET original_email='',email_key='' WHERE subject_id=$1`,
			`UPDATE identity_personal_emails SET original_email='用户@example.invalid',email_key='用户@example.invalid' WHERE subject_id=$1`,
			`INSERT INTO identity_personal_emails(subject_id,email_key,original_email) VALUES(repeat('f',32),'fk@example.invalid','fk@example.invalid')`,
			`DELETE FROM identity_credentials WHERE id=$1`,
		} {
			// The FK fixture has no parameter; pgx uses the extended protocol when
			// arguments exist, so bind only queries that actually accept one.
			var err error
			if strings.Contains(query, "$1") {
				_, err = a.Exec(ctx, query, identityID(1810))
			} else {
				_, err = a.Exec(ctx, query)
			}
			if err == nil {
				t.Fatal("database accepted inconsistent mapping or deleted its subject")
			}
		}
		_, err := a.Exec(ctx, `UPDATE identity_personal_emails SET original_email='not-an-address',email_key='not-an-address' WHERE subject_id=$1`, identityID(1810))
		must(t, err)
		if _, err := first.PersonalByID(ctx, identityID(1810)); !errors.Is(err, identity.ErrStore) {
			t.Fatal("malformed trusted-writer metadata became an accepted email")
		}
		_, err = a.Exec(ctx, `UPDATE identity_personal_emails SET original_email=$2,email_key=$3 WHERE subject_id=$1`, identityID(1810), email.Original(), email.Key())
		must(t, err)
	})
	t.Run("natural email and subject lookup plans", func(t *testing.T) { testPersonalEmailPlans(t, ctx, a) })
}

func testPersonalCompetition(t *testing.T, ctx context.Context, first, second *identitypg.Store, r password.Record, sameID bool) {
	t.Helper()
	ids := []string{identityID(1830), identityID(1831)}
	emails := []identity.Email{personalEmail(t, "Race@Example.invalid"), personalEmail(t, "race@example.INVALID")}
	if sameID {
		ids = []string{identityID(1840), identityID(1840)}
		emails = []identity.Email{personalEmail(t, "race-one@example.invalid"), personalEmail(t, "race-two@example.invalid")}
	}
	type result struct {
		index int
		row   identity.Personal
		err   error
	}
	start, results := make(chan struct{}), make(chan result, 2)
	for i, store := range []*identitypg.Store{first, second} {
		go func() {
			<-start
			row, err := store.InitializePersonal(ctx, ids[i], emails[i], true, r)
			results <- result{i, row, err}
		}()
	}
	close(start)
	winner, loser := <-results, <-results
	if winner.err != nil {
		winner, loser = loser, winner
	}
	if winner.err != nil || loser.err != identity.ErrConflict || loser.row != (identity.Personal{}) {
		t.Fatal("concurrent initialization did not produce exactly one owner")
	}
	got, err := second.PersonalByID(ctx, ids[winner.index])
	must(t, err)
	if got != winner.row || got.Email != emails[winner.index] {
		t.Fatal("winning metadata was overwritten")
	}
	if !sameID {
		if _, err := second.Get(ctx, ids[loser.index]); err != identity.ErrNotFound {
			t.Fatal("losing email initializer left an orphan subject")
		}
	} else if _, err := second.LookupPersonalByEmail(ctx, emails[loser.index]); err != identity.ErrNotFound {
		t.Fatal("losing subject initializer attached another email")
	}
}

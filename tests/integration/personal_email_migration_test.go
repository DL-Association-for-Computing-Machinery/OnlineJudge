//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func personalEmail(t *testing.T, raw string) identity.Email {
	t.Helper()
	email, err := identity.ParseEmail(raw)
	must(t, err)
	return email
}

func testPersonalEmailMigrations(t *testing.T, ctx context.Context, a, b *pgxpool.Pool, provider *goose.Provider, submissions *submissionpg.Store, h *password.Hasher) {
	store, err := identitypg.New(a, h)
	must(t, err)
	r := identityRecord(t, h, "synthetic personal migration")
	t.Run("four to five preserves state and empty mapping rollback", func(t *testing.T) {
		_, err := provider.Down(ctx)
		must(t, err)
		requireRecoverySchemaFour(t, ctx, a)
		account, err := store.Initialize(ctx, identityID(800), false, r)
		must(t, err)
		work, _, err := submissions.Accept(ctx, "synthetic-pre-email", "training", "personal-upgrade-existing-01", submission.Request{
			MaterialVersion: experiment.Version, Language: "gnu++17", Source: "// synthetic retained email migration source"})
		must(t, err)
		capture := func() string {
			var snapshot string
			must(t, a.QueryRow(ctx, `SELECT jsonb_build_object('identity',row_to_json(i),'recovery',row_to_json(r),
                'submission',row_to_json(s),'task',row_to_json(t))::text FROM identity_credentials i,identity_session_recovery r,
                submissions s JOIN judge_tasks t ON t.submission_id=s.id WHERE i.id=$1 AND s.id=$2`, account.ID, work.ID).Scan(&snapshot))
			return snapshot
		}
		before := capture()
		_, err = provider.Up(ctx)
		must(t, err)
		must(t, db.CheckSchema(ctx, a))
		if capture() != before {
			t.Fatal("email migration changed existing business or recovery state")
		}
		if _, err := store.PersonalByID(ctx, account.ID); err != identity.ErrNotFound {
			t.Fatal("migration invented an email for a generic subject")
		}
		_, err = provider.Down(ctx)
		must(t, err)
		requireRecoverySchemaFour(t, ctx, a)
		if capture() != before {
			t.Fatal("empty mapping rollback changed existing state")
		}
		var absent bool
		must(t, a.QueryRow(ctx, `SELECT to_regclass('identity_personal_emails') IS NULL`).Scan(&absent))
		if !absent {
			t.Fatal("mapping table survived rollback")
		}
		_, err = provider.Up(ctx)
		must(t, err)
		row, err := store.InitializePersonal(ctx, identityID(801), personalEmail(t, "Original@Example.invalid"), false, r)
		must(t, err)
		if _, err := provider.Down(ctx); err == nil {
			t.Fatal("rollback discarded an unverified, disabled email mapping")
		}
		must(t, db.CheckSchema(ctx, a))
		got, err := store.PersonalByID(ctx, row.ID)
		must(t, err)
		if got != row || capture() != before {
			t.Fatal("refused rollback changed retained state")
		}
		_, err = a.Exec(ctx, `DELETE FROM identity_personal_emails WHERE subject_id=$1;`, row.ID)
		must(t, err)
		_, err = a.Exec(ctx, `DELETE FROM identity_credentials WHERE id=$1`, row.ID)
		must(t, err)
		_, err = a.Exec(ctx, `DELETE FROM identity_credentials WHERE id=$1`, account.ID)
		must(t, err)
		_, err = a.Exec(ctx, `DELETE FROM judge_tasks WHERE submission_id=$1`, work.ID)
		must(t, err)
		_, err = a.Exec(ctx, `DELETE FROM submissions WHERE id=$1`, work.ID)
		must(t, err)
	})
	t.Run("rollback serializes actual personal initialization", func(t *testing.T) {
		_, err := a.Exec(ctx, `CREATE FUNCTION wait_personal_commit() RETURNS trigger LANGUAGE plpgsql AS $$
            BEGIN PERFORM pg_advisory_xact_lock(91465066); RETURN NEW; END $$;
            CREATE FUNCTION wait_personal_rollback() RETURNS trigger LANGUAGE plpgsql AS $$
            BEGIN PERFORM pg_advisory_xact_lock(91465066); RAISE EXCEPTION 'synthetic personal insert rejected' USING ERRCODE='P0001'; END $$`)
		must(t, err)
		defer func() {
			_, err := a.Exec(ctx, `DROP FUNCTION wait_personal_commit(); DROP FUNCTION wait_personal_rollback()`)
			must(t, err)
		}()
		for _, table := range []string{"identity_personal_emails", "identity_credentials"} {
			for _, commit := range []bool{false, true} {
				t.Run(table+"/"+map[bool]string{false: "failed initialization permits rollback", true: "committed initialization refuses rollback"}[commit], func(t *testing.T) {
					function := "wait_personal_rollback"
					if commit {
						function = "wait_personal_commit"
					}
					// Table/function names are fixed fixture constants, never external input.
					_, err := a.Exec(ctx, `CREATE TRIGGER wait_personal AFTER INSERT ON `+table+` FOR EACH ROW EXECUTE FUNCTION `+function+`()`)
					must(t, err)
					held, err := b.Acquire(ctx)
					must(t, err)
					defer held.Release()
					_, err = held.Exec(ctx, `SELECT pg_advisory_lock(91465066)`)
					must(t, err)
					defer func() { _, _ = held.Exec(ctx, `SELECT pg_advisory_unlock(91465066)`) }()
					var pid int32
					must(t, held.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
					email := personalEmail(t, "migration-race@example.invalid")
					initialized := make(chan error, 1)
					go func() { _, err := store.InitializePersonal(ctx, identityID(802), email, true, r); initialized <- err }()
					writer := blockedIdentityBackend(t, ctx, b, pid)
					down := make(chan error, 1)
					go func() { _, err := provider.Down(ctx); down <- err }()
					waitIdentityBlocked(t, ctx, b, writer.pid)
					_, err = held.Exec(ctx, `SELECT pg_advisory_unlock(91465066)`)
					must(t, err)
					initErr, downErr := awaitAuthority(t, initialized), awaitAuthority(t, down)
					if commit {
						must(t, initErr)
						var cause *pgconn.PgError
						if !errors.As(downErr, &cause) || cause.Code != "P0001" {
							t.Fatalf("rollback did not refuse retained email: %v", downErr)
						}
						must(t, db.CheckSchema(ctx, a))
						_, err = a.Exec(ctx, `DROP TRIGGER wait_personal ON `+table)
						must(t, err)
						_, err = a.Exec(ctx, `DELETE FROM identity_personal_emails WHERE subject_id=$1`, identityID(802))
						must(t, err)
					} else {
						if !errors.Is(initErr, identity.ErrStore) {
							t.Fatal("failed initializer hid database error")
						}
						must(t, downErr)
						requireRecoverySchemaFour(t, ctx, a)
						if _, err := store.Get(ctx, identityID(802)); err != identity.ErrNotFound {
							t.Fatal("rolled-back initializer left orphan credentials")
						}
						if table == "identity_credentials" {
							_, err = a.Exec(ctx, `DROP TRIGGER wait_personal ON identity_credentials`)
							must(t, err)
						}
						_, err = provider.Up(ctx)
						must(t, err)
					}
					_, err = a.Exec(ctx, `DELETE FROM identity_credentials WHERE id=$1`, identityID(802))
					must(t, err)
				})
			}
		}
	})
}

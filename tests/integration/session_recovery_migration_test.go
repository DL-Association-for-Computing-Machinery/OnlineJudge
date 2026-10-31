//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"

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

func testSessionRecoveryMigrations(t *testing.T, ctx context.Context, a, b *pgxpool.Pool, provider *goose.Provider, submissions *submissionpg.Store, h *password.Hasher) {
	// Exercise migration 4 itself; migration 5 must not mask its rollback guard.
	_, err := provider.DownTo(ctx, 4)
	must(t, err)
	defer func() { _, err := provider.Up(ctx); must(t, err) }()
	store, err := identitypg.New(a, h)
	must(t, err)
	t.Run("schema three upgrade preserves complete business data", func(t *testing.T) {
		_, err := provider.DownTo(ctx, 3)
		must(t, err)
		requireIdentitySchemaThree(t, ctx, a)
		account, err := store.Initialize(ctx, identityID(700), true, identityRecord(t, h, "synthetic retained password"))
		must(t, err)
		work, _, err := submissions.Accept(ctx, "synthetic-pre-recovery", "training", "recovery-upgrade-existing-01", submission.Request{
			MaterialVersion: experiment.Version, Language: "gnu++17", Source: "// synthetic retained version-three source"})
		must(t, err)
		_, err = a.Exec(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES('pre-recovery-node',1)`)
		must(t, err)
		_, err = a.Exec(ctx, `UPDATE judge_tasks SET state='running',attempt_number=1 WHERE submission_id=$1`, work.ID)
		must(t, err)
		_, err = a.Exec(ctx, `INSERT INTO judge_attempts(token,submission_id,judge_generation,recovery_generation,attempt_number,node_id,credential_version,lease_until)
            VALUES('pre-recovery-lease',$1,1,1,1,'pre-recovery-node',1,clock_timestamp()+interval '1 hour')`, work.ID)
		must(t, err)
		capture := func() string {
			var snapshot string
			must(t, a.QueryRow(ctx, `SELECT jsonb_build_object('identity',row_to_json(i),'submission',row_to_json(s),'task',row_to_json(t),'attempt',row_to_json(x))::text
                FROM identity_credentials i,submissions s JOIN judge_tasks t ON t.submission_id=s.id
                JOIN judge_attempts x ON x.submission_id=s.id WHERE i.id=$1 AND s.id=$2`, account.ID, work.ID).Scan(&snapshot))
			return snapshot
		}
		before := capture()
		_, err = provider.UpTo(ctx, 4)
		must(t, err)
		requireRecoverySchemaFour(t, ctx, a)
		if capture() != before {
			t.Fatal("recovery migration changed identity, source, task, or lease")
		}
		if _, err := provider.Down(ctx); err == nil || capture() != before {
			t.Fatal("rollback ignored retained business data or changed it")
		}
		_, err = a.Exec(ctx, `DELETE FROM judge_attempts WHERE submission_id=$1`, work.ID)
		must(t, err)
		_, err = a.Exec(ctx, `DELETE FROM judge_tasks WHERE submission_id=$1`, work.ID)
		must(t, err)
		_, err = a.Exec(ctx, `DELETE FROM submissions WHERE id=$1`, work.ID)
		must(t, err)
		_, err = a.Exec(ctx, `DELETE FROM worker_nodes WHERE id='pre-recovery-node'`)
		must(t, err)
		_, err = a.Exec(ctx, `DELETE FROM identity_credentials WHERE id=$1`, account.ID)
		must(t, err)
	})
	for _, credentials := range []bool{true, false} {
		t.Run(map[bool]string{true: "credentials alone prevent rollback", false: "submissions alone prevent rollback"}[credentials], func(t *testing.T) {
			id := identityID(701)
			if credentials {
				row, err := store.Initialize(ctx, id, true, identityRecord(t, h, "synthetic retained password"))
				must(t, err)
				if _, err := provider.Down(ctx); err == nil {
					t.Fatal("rollback discarded credentials without accepted work")
				}
				sameIdentity(t, ctx, store, row)
				_, err = a.Exec(ctx, `DELETE FROM identity_credentials WHERE id=$1`, id)
				must(t, err)
			} else {
				work, _, err := submissions.Accept(ctx, "synthetic-recovery-rollback", "training", "recovery-rollback-sub-only-01", submission.Request{
					MaterialVersion: experiment.Version, Language: "gnu++17", Source: "// synthetic submission-only rollback fixture"})
				must(t, err)
				capture := func() string {
					var snapshot string
					must(t, a.QueryRow(ctx, `SELECT jsonb_build_object('submission',row_to_json(s),'task',row_to_json(t))::text
                        FROM submissions s JOIN judge_tasks t ON t.submission_id=s.id WHERE s.id=$1`, work.ID).Scan(&snapshot))
					return snapshot
				}
				before := capture()
				if _, err := provider.Down(ctx); err == nil || capture() != before {
					t.Fatal("rollback discarded or changed accepted work without credentials")
				}
				_, err = a.Exec(ctx, `DELETE FROM judge_tasks WHERE submission_id=$1`, work.ID)
				must(t, err)
				_, err = a.Exec(ctx, `DELETE FROM submissions WHERE id=$1`, work.ID)
				must(t, err)
			}
			requireRecoverySchemaFour(t, ctx, a)
		})
	}
	t.Run("closed placeholder constraints and missing authority", func(t *testing.T) {
		state, err := store.RecoveryState(ctx)
		must(t, err)
		if state != (identity.RecoveryState{}) {
			t.Fatal("migration did not bootstrap a closed empty placeholder")
		}
		calls := 0
		err = store.WithSession(ctx, identityID(701), identity.SessionVersion{AuthGeneration: 1, RecoveryID: identityID(1)}, func(context.Context, identity.SessionState) error { calls++; return nil })
		mustCredentialError(t, err, identity.ErrRecovering)
		if calls != 0 {
			t.Fatal("uninitialized recovery admitted an operation")
		}
		for _, query := range []string{
			`UPDATE identity_session_recovery SET generation='invalid'`,
			`UPDATE identity_session_recovery SET generation=repeat('A',32)`,
			`UPDATE identity_session_recovery SET enabled=true`,
			`INSERT INTO identity_session_recovery(singleton) VALUES(false)`,
			`INSERT INTO identity_session_recovery(singleton) VALUES(true)`,
		} {
			if _, err := a.Exec(ctx, query); err == nil {
				t.Fatal("recovery table accepted an invalid closed/open singleton")
			}
		}
		_, err = a.Exec(ctx, `DELETE FROM identity_session_recovery`)
		must(t, err)
		_, err = store.RecoveryState(ctx)
		mustCredentialError(t, err, identity.ErrRecovering)
		generation, err := store.BeginRecovery(ctx)
		if generation != "" || !errors.Is(err, identity.ErrRecovering) {
			t.Fatal("missing recovery state repaired itself")
		}
		if _, err := provider.Down(ctx); err == nil {
			t.Fatal("missing recovery authority was treated as never initialized")
		}
		// Only this disposable migration fixture restores the bootstrap row.
		_, err = a.Exec(ctx, `INSERT INTO identity_session_recovery(singleton) VALUES(true)`)
		must(t, err)
		_, err = provider.Down(ctx)
		must(t, err)
		requireIdentitySchemaThree(t, ctx, a)
		_, err = provider.UpTo(ctx, 4)
		must(t, err)
		requireRecoverySchemaFour(t, ctx, a)
	})
	t.Run("rollback serializes actual recovery begin", func(t *testing.T) {
		testRecoveryMigrationBegin(t, ctx, a, b, provider, store)
	})
}

func testRecoveryMigrationBegin(t *testing.T, ctx context.Context, a, b *pgxpool.Pool, provider *goose.Provider, store *identitypg.Store) {
	_, err := a.Exec(ctx, `CREATE FUNCTION wait_recovery_commit() RETURNS trigger LANGUAGE plpgsql AS $$
        BEGIN PERFORM pg_advisory_xact_lock(91465065); RETURN NEW; END $$;
        CREATE FUNCTION wait_recovery_rollback() RETURNS trigger LANGUAGE plpgsql AS $$
        BEGIN PERFORM pg_advisory_xact_lock(91465065); RAISE EXCEPTION 'synthetic recovery update rejected' USING ERRCODE='P0001'; END $$`)
	must(t, err)
	defer func() {
		_, err := a.Exec(ctx, `DROP FUNCTION wait_recovery_commit(); DROP FUNCTION wait_recovery_rollback()`)
		must(t, err)
	}()
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "failed begin permits rollback", true: "committed begin prevents rollback"}[commit], func(t *testing.T) {
			trigger := `CREATE TRIGGER wait_recovery AFTER UPDATE ON identity_session_recovery FOR EACH ROW EXECUTE FUNCTION wait_recovery_rollback()`
			if commit {
				trigger = `CREATE TRIGGER wait_recovery AFTER UPDATE ON identity_session_recovery FOR EACH ROW EXECUTE FUNCTION wait_recovery_commit()`
			}
			_, err := a.Exec(ctx, trigger)
			must(t, err)
			held, err := b.Acquire(ctx)
			must(t, err)
			defer held.Release()
			_, err = held.Exec(ctx, `SELECT pg_advisory_lock(91465065)`)
			must(t, err)
			defer func() { _, _ = held.Exec(ctx, `SELECT pg_advisory_unlock(91465065)`) }()
			var pid int32
			must(t, held.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
			begin := make(chan recoveryBeginResult, 1)
			go func() { generation, err := store.BeginRecovery(ctx); begin <- recoveryBeginResult{generation, err} }()
			writer := blockedIdentityBackend(t, ctx, b, pid)
			down := make(chan error, 1)
			go func() { _, err := provider.Down(ctx); down <- err }()
			waitIdentityBlocked(t, ctx, b, writer.pid)
			var unlocked bool
			must(t, held.QueryRow(ctx, `SELECT pg_advisory_unlock(91465065)`).Scan(&unlocked))
			if !unlocked {
				t.Fatal("fixture did not release recovery begin")
			}
			result := awaitRecoveryBegin(t, begin)
			err = awaitAuthority(t, down)
			if commit {
				must(t, result.err)
				if !identity.ValidRecoveryID(result.generation) || err == nil {
					t.Fatal("rollback discarded a committed recovery epoch")
				}
				state, err := store.RecoveryState(ctx)
				must(t, err)
				if state != (identity.RecoveryState{Generation: result.generation}) {
					t.Fatal("refused rollback changed recovery state")
				}
			} else {
				if result.generation != "" || !errors.Is(result.err, identity.ErrStore) {
					t.Fatal("failed begin returned a generation or hid storage failure")
				}
				must(t, err)
				requireIdentitySchemaThree(t, ctx, a)
				_, err = provider.UpTo(ctx, 4)
				must(t, err)
			}
			_, err = a.Exec(ctx, `DROP TRIGGER IF EXISTS wait_recovery ON identity_session_recovery`)
			must(t, err)
			requireRecoverySchemaFour(t, ctx, a)
		})
	}
}

func requireRecoverySchemaFour(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var version int64
	must(t, pool.QueryRow(ctx, `SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1`).Scan(&version))
	if version != 4 || db.CheckSchema(ctx, pool) == nil {
		t.Fatal("expected schema four, rejected by the schema-five API")
	}
}

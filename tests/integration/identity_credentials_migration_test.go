//go:build integration

package integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func testIdentityMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool, provider *goose.Provider, submissions *submissionpg.Store, h *password.Hasher) {
	t.Run("upgrade version two retains accepted work and lease", func(t *testing.T) {
		_, err := provider.DownTo(ctx, 2)
		must(t, err)
		if db.CheckSchema(ctx, pool) == nil {
			t.Fatal("new API accepted schema two")
		}
		row, _, err := submissions.Accept(ctx, "synthetic-pre-identity", "training", "identity-upgrade-existing-01", submission.Request{
			MaterialVersion: experiment.Version, Language: "gnu++17", Source: "// synthetic v2 retained source"})
		must(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES('pre-identity-node',1)`)
		must(t, err)
		_, err = pool.Exec(ctx, `UPDATE judge_tasks SET state='running',attempt_number=1 WHERE submission_id=$1`, row.ID)
		must(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO judge_attempts(token,submission_id,judge_generation,recovery_generation,attempt_number,node_id,credential_version,lease_until)
            VALUES('pre-identity-lease',$1,1,1,1,'pre-identity-node',1,clock_timestamp()+interval '1 hour')`, row.ID)
		must(t, err)
		capture := func() string {
			var state string
			must(t, pool.QueryRow(ctx, `SELECT jsonb_build_object('source',row_to_json(s),'task',row_to_json(t),'attempt',row_to_json(a))::text
                FROM submissions s JOIN judge_tasks t ON t.submission_id=s.id JOIN judge_attempts a ON a.submission_id=s.id WHERE s.id=$1`, row.ID).Scan(&state))
			return state
		}
		before := capture()
		_, err = provider.Up(ctx)
		must(t, err)
		must(t, db.CheckSchema(ctx, pool))
		if capture() != before {
			t.Fatal("migration changed accepted source, task, or lease")
		}
		if _, err := provider.Down(ctx); err == nil {
			t.Fatal("rollback discarded schema after accepted work")
		}
		must(t, db.CheckSchema(ctx, pool))
		if capture() != before {
			t.Fatal("refused rollback changed accepted work")
		}
		_, err = pool.Exec(ctx, `DELETE FROM judge_attempts WHERE submission_id=$1`, row.ID)
		must(t, err)
		_, err = pool.Exec(ctx, `DELETE FROM judge_tasks WHERE submission_id=$1`, row.ID)
		must(t, err)
		_, err = pool.Exec(ctx, `DELETE FROM submissions WHERE id=$1`, row.ID)
		must(t, err)
		_, err = pool.Exec(ctx, `DELETE FROM worker_nodes WHERE id='pre-identity-node'`)
		must(t, err)
	})
	for _, commit := range []bool{true, false} {
		name := "rollback waits for initializing rollback"
		if commit {
			name = "rollback waits for initializing commit"
		}
		t.Run(name, func(t *testing.T) {
			short, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			r := identityRecord(t, h, "synthetic initializing password")
			p := r.Parameters
			tx, err := pool.Begin(short)
			must(t, err)
			defer tx.Rollback(ctx)
			var pid int32
			must(t, tx.QueryRow(short, `SELECT pg_backend_pid()`).Scan(&pid))
			_, err = tx.Exec(short, `INSERT INTO identity_credentials
                (id,enabled,format_version,algorithm,argon_version,memory_kib,iterations,parallelism,salt_bytes,digest_bytes,salt,digest)
                VALUES($1,true,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, identityID(1), int32(r.FormatVersion), r.Algorithm, int32(r.ArgonVersion),
				int64(p.MemoryKiB), int64(p.Iterations), int16(p.Parallelism), int32(p.SaltBytes), int32(p.DigestBytes), r.Salt, r.Digest)
			must(t, err)
			result := make(chan error, 1)
			go func() { _, err := provider.Down(short); result <- err }()
			waitIdentityBlocked(t, short, pool, pid)
			if commit {
				must(t, tx.Commit(short))
			} else {
				must(t, tx.Rollback(short))
			}
			err = <-result
			if commit {
				if err == nil {
					t.Fatal("rollback removed newly committed identity")
				}
				must(t, db.CheckSchema(ctx, pool))
				store, err := identitypg.New(pool, h)
				must(t, err)
				got, err := store.Get(ctx, identityID(1))
				must(t, err)
				if !got.Enabled || got.CredentialVersion != 1 || got.AuthGeneration != 1 {
					t.Fatal("rollback changed initializing account")
				}
				_, err = pool.Exec(ctx, `DELETE FROM identity_credentials WHERE id=$1`, got.ID)
				must(t, err)
			} else {
				must(t, err)
				if db.CheckSchema(ctx, pool) == nil {
					t.Fatal("removed identity schema still accepted")
				}
				_, err = provider.Up(ctx)
				must(t, err)
				must(t, db.CheckSchema(ctx, pool))
			}
		})
	}
}

func testIdentityPlans(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	// Reuse a synthetic digest only as cardinality padding; these are not issued
	// user credentials and do not measure KDF or production account capacity.
	_, err := pool.Exec(ctx, `INSERT INTO identity_credentials
        SELECT lpad(to_hex(n),32,'0'),true,1,1,format_version,algorithm,argon_version,memory_kib,iterations,parallelism,salt_bytes,digest_bytes,salt,digest
        FROM identity_credentials CROSS JOIN generate_series(5000,6999) n WHERE id=$1`, identityID(101))
	must(t, err)
	_, err = pool.Exec(ctx, `ANALYZE identity_credentials`)
	must(t, err)
	tx, err := pool.Begin(ctx)
	must(t, err)
	defer tx.Rollback(ctx)
	for _, query := range []string{
		`EXPLAIN (ANALYZE,BUFFERS) SELECT * FROM identity_credentials WHERE id=$1`,
		`EXPLAIN (ANALYZE,BUFFERS) UPDATE identity_credentials SET credential_version=credential_version+1
            WHERE id=$1 AND enabled AND credential_version=1 AND auth_generation=1 AND credential_version<9223372036854775807 RETURNING id`,
	} {
		rows, err := tx.Query(ctx, query, identityID(5000))
		must(t, err)
		var plan strings.Builder
		for rows.Next() {
			var line string
			must(t, rows.Scan(&line))
			plan.WriteString(line + "\n")
		}
		must(t, rows.Err())
		rows.Close()
		if !strings.Contains(plan.String(), "identity_credentials_pkey") || !strings.Contains(plan.String(), "Index") {
			t.Fatal("point lookup or CAS lacked natural primary-key plan")
		}
		t.Log(plan.String())
	}
	var size int64
	must(t, tx.QueryRow(ctx, `SELECT pg_relation_size('identity_credentials_pkey')`).Scan(&size))
	t.Logf("Synthetic fixture primary-key index bytes: %d; not a production capacity estimate", size)
}

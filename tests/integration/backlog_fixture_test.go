//go:build integration

package integration_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

type backlogFixture struct {
	ctx           context.Context
	a, b          *pgxpool.Pool
	first, second *postgres.Store
	provider      *goose.Provider
	request       submission.Request
	appA, appB    string
}

func newBacklogFixture(t *testing.T) backlogFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	t.Cleanup(cancel)
	cfg, err := pgxpool.ParseConfig(os.Getenv("OJ_TEST_DATABASE_URL"))
	must(t, err)
	if cfg.ConnConfig.User != "oj_test" || !strings.HasPrefix(cfg.ConnConfig.Host, "/tmp/oj-postgres.") {
		t.Fatal("use the toolchain's private disposable PostgreSQL cluster")
	}
	admin, err := postgres.Open(ctx, cfg.ConnString())
	must(t, err)
	t.Cleanup(admin.Close)
	var random [12]byte
	_, err = rand.Read(random[:])
	must(t, err)
	schema := "backlog_" + hex.EncodeToString(random[:])
	identifier := pgx.Identifier{schema}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+identifier)
	must(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error("cannot clean the owned backlog test schema")
		}
	})
	// ConnString preserves the original parsed DSN, not subsequent config edits.
	// Pass typed copies to both pool and stdlib rather than losing search_path.
	cfg = admin.Config()
	cfg.ConnConfig.RuntimeParams["search_path"] = identifier
	f := backlogFixture{ctx: ctx, appA: schema + "_a", appB: schema + "_b"}
	conn := stdlib.OpenDB(*cfg.ConnConfig.Copy())
	conn.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = conn.Close() })
	var actual string
	must(t, conn.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&actual))
	if actual != schema {
		t.Fatal("migration connection escaped the owned test schema")
	}
	f.provider, err = db.Provider(conn)
	must(t, err)
	_, err = f.provider.Up(ctx)
	must(t, err)
	var ownTables int
	must(t, conn.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name IN ('goose_db_version','submission_backlog_policy','submissions','judge_tasks')`, schema).Scan(&ownTables))
	if ownTables != 4 {
		t.Fatal("migrations did not remain in the owned test schema")
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = f.appA
	f.a, err = pgxpool.NewWithConfig(ctx, cfg.Copy())
	must(t, err)
	t.Cleanup(f.a.Close)
	cfg.ConnConfig.RuntimeParams["application_name"] = f.appB
	f.b, err = pgxpool.NewWithConfig(ctx, cfg.Copy())
	must(t, err)
	t.Cleanup(f.b.Close)
	for _, pool := range []*pgxpool.Pool{f.a, f.b} {
		must(t, pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&actual))
		if actual != schema {
			t.Fatal("admission client escaped the owned test schema")
		}
	}
	f.first, err = postgres.New(f.a, experiment.Version, experiment.Manifest())
	must(t, err)
	f.second, err = postgres.New(f.b, experiment.Version, experiment.Manifest())
	must(t, err)
	must(t, f.first.RegisterFixture(ctx))
	f.request = submission.Request{MaterialVersion: experiment.Version, Language: "gnu++17", Source: "// synthetic backlog; not executed\n"}
	return f
}

func backlogPolicy(t *testing.T, f backlogFixture, global, subject int64) {
	t.Helper()
	must(t, postgres.ConfigureBacklog(f.ctx, f.a, postgres.BacklogLimits{Global: global, Subject: subject}))
}

func backlogCounts(t *testing.T, f backlogFixture, want int) {
	t.Helper()
	var submissions, tasks int
	must(t, f.a.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM submissions),(SELECT count(*) FROM judge_tasks)`).Scan(&submissions, &tasks))
	if submissions != want || tasks != want {
		t.Fatalf("source/task counts=%d/%d want=%d", submissions, tasks, want)
	}
}

func waitBacklogBlocked(t *testing.T, f backlogFixture, holder uint32, app string) uint32 {
	t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, 2*time.Second)
	defer cancel()
	for {
		var pid uint32
		err := f.a.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)) AND application_name=$2 LIMIT 1`, holder, app).Scan(&pid)
		if err == nil {
			return pid
		}
		if ctx.Err() != nil {
			t.Fatal("expected actual database lock wait not observed")
		}
		select {
		case <-ctx.Done():
			t.Fatal("database lock observation expired")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

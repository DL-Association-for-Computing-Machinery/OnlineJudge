//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

func TestBacklogLifecycle(t *testing.T) {
	for _, lost := range []bool{false, true} {
		name := "system result exhaustion"
		if lost {
			name = "lost node exhaustion"
		}
		t.Run(name, func(t *testing.T) {
			f := newBacklogFixture(t)
			backlogPolicy(t, f, 1, 1)
			row, _, err := f.first.Accept(f.ctx, "user", "training", "backlog-lifecycle-01", f.request)
			must(t, err)
			node := submission.NodeIdentity{ID: "backlog-node", CredentialVersion: 1}
			_, err = f.a.Exec(f.ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)`, node.ID)
			must(t, err)
			for attempt := 1; attempt <= 3; attempt++ {
				lease, err := f.first.Claim(f.ctx, node)
				must(t, err)
				if lease.SubmissionID != row.ID || lease.AttemptNumber != int32(attempt) {
					t.Fatal("wrong attempt")
				}
				_, _, err = f.second.Accept(f.ctx, "other", "training", "backlog-lifecycle-next", f.request)
				if !errors.Is(err, submission.ErrBacklog) {
					t.Fatal("claim incorrectly released backlog capacity")
				}
				if lost {
					_, err = f.a.Exec(f.ctx, `UPDATE judge_attempts SET lease_until=clock_timestamp()-interval '1 second' WHERE token=$1`, lease.Token)
					must(t, err)
					n, err := f.second.Recover(f.ctx)
					must(t, err)
					if n != 1 {
						t.Fatal("expected one recovered attempt")
					}
				} else {
					result := submission.Result{Verdict: "SE", EvidenceSHA256: strings.Repeat("b", 64)}
					must(t, f.first.Report(f.ctx, node, lease.Token, result))
					must(t, f.second.Report(f.ctx, node, lease.Token, result))
				}
				if attempt < 3 {
					_, _, err = f.second.Accept(f.ctx, "other", "training", "backlog-lifecycle-next", f.request)
					if !errors.Is(err, submission.ErrBacklog) {
						t.Fatal("requeue incorrectly released capacity")
					}
				}
			}
			failed, err := f.second.Get(f.ctx, "user", "training", row.ID)
			must(t, err)
			if failed.State != "failed" || failed.Verdict != nil {
				t.Fatal("system exhaustion fabricated contestant result")
			}
			next, _, err := f.second.Accept(f.ctx, "other", "training", "backlog-lifecycle-next", f.request)
			must(t, err)
			lease, err := f.first.Claim(f.ctx, node)
			must(t, err)
			if lease.SubmissionID != next.ID {
				t.Fatal("wrong next task")
			}
			ac := submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("a", 64)}
			must(t, f.first.Report(f.ctx, node, lease.Token, ac))
			must(t, f.second.Report(f.ctx, node, lease.Token, ac))
			_, _, err = f.first.Accept(f.ctx, "third", "training", "backlog-after-terminal", f.request)
			must(t, err)
			_, _, err = f.second.Accept(f.ctx, "fourth", "training", "backlog-terminal-full", f.request)
			if !errors.Is(err, submission.ErrBacklog) {
				t.Fatal("duplicate report released capacity twice")
			}
			backlogCounts(t, f, 3)
		})
	}
}

func TestBacklogMigrationGuard(t *testing.T) {
	t.Run("empty rollback and configured policy protection", func(t *testing.T) {
		f := newBacklogFixture(t)
		// Exercise the migration that owns backlog policy, independently of
		// newer additive migrations whose empty rollback is permitted.
		_, err := f.provider.DownTo(f.ctx, 6)
		must(t, err)
		_, err = f.provider.Down(f.ctx)
		must(t, err)
		if db.CheckSchema(f.ctx, f.a) == nil {
			t.Fatal("new API accepted old schema")
		}
		_, err = f.provider.UpTo(f.ctx, 6)
		must(t, err)
		backlogPolicy(t, f, 2, 1)
		if _, err = f.provider.Down(f.ctx); err == nil {
			t.Fatal("rollback discarded configured policy")
		}
		requireBacklogSchemaSix(t, f)
		var global, subject int64
		must(t, f.a.QueryRow(f.ctx, `SELECT global_limit,subject_limit FROM submission_backlog_policy`).Scan(&global, &subject))
		if global != 2 || subject != 1 {
			t.Fatal("failed rollback changed policy")
		}
		_, _, err = f.first.Accept(f.ctx, "user", "training", "backlog-down-retained", f.request)
		must(t, err)
		_, err = f.a.Exec(f.ctx, `DELETE FROM submission_backlog_policy`)
		must(t, err)
		if _, err = f.provider.Down(f.ctx); err == nil {
			t.Fatal("rollback discarded accepted work without policy")
		}
		backlogCounts(t, f, 1)
	})
	for _, commit := range []bool{false, true} {
		t.Run(fmt.Sprintf("configuration race commit=%v", commit), func(t *testing.T) {
			f := newBacklogFixture(t)
			_, err := f.provider.DownTo(f.ctx, 6)
			must(t, err)
			held, err := f.a.Begin(f.ctx)
			must(t, err)
			defer held.Rollback(context.Background())
			_, err = held.Exec(f.ctx, `INSERT INTO submission_backlog_policy VALUES(true,2,1)`)
			must(t, err)
			result := make(chan error, 1)
			go func() { _, err := f.provider.Down(f.ctx); result <- err }()
			waitBacklogBlocked(t, f, held.Conn().PgConn().PID(), "")
			if commit {
				must(t, held.Commit(f.ctx))
			} else {
				must(t, held.Rollback(f.ctx))
			}
			err = <-result
			if commit {
				if err == nil {
					t.Fatal("rollback lost committed configuration")
				}
				requireBacklogSchemaSix(t, f)
			} else {
				must(t, err)
				if db.CheckSchema(f.ctx, f.a) == nil {
					t.Fatal("expected old schema after safe rollback")
				}
			}
		})
	}
}

func requireBacklogSchemaSix(t *testing.T, f backlogFixture) {
	t.Helper()
	var version int
	must(t, f.a.QueryRow(f.ctx, `SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1`).Scan(&version))
	if version != 6 {
		t.Fatalf("backlog migration changed on refused rollback: %d", version)
	}
}

func TestBacklogQueryPlan(t *testing.T) {
	f := newBacklogFixture(t)
	backlogPolicy(t, f, 10000, 10000)
	_, err := f.a.Exec(f.ctx, `INSERT INTO submissions(id,subject_id,context_id,idempotency_key,request_sha256,material_version,manifest_sha256,language,source_code)
        SELECT md5('history-'||n::text),'history','training','history-fixture-'||n::text,$1,m.version_id,m.manifest_sha256,'gnu++17',$2
        FROM generate_series(1,2048) n CROSS JOIN submission_materials m WHERE m.version_id=$3`, submission.Digest(f.request), f.request.Source, f.request.MaterialVersion)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `INSERT INTO judge_tasks(submission_id,state,verdict) SELECT id,'finished','AC' FROM submissions WHERE subject_id='history'`)
	must(t, err)
	for i := 0; i < 3; i++ {
		_, _, err = f.first.Accept(f.ctx, "active", "training", fmt.Sprintf("backlog-plan-active-%02d", i), f.request)
		must(t, err)
	}
	_, err = f.a.Exec(f.ctx, `ANALYZE judge_tasks; ANALYZE submissions`)
	must(t, err)
	// Explain the authoritative sqlc source, rather than a second hand-copied query.
	raw, err := os.ReadFile("../../db/queries/submissions.sql")
	must(t, err)
	parts := strings.Split(string(raw), "-- name: CountBacklog :one\n")
	if len(parts) != 2 {
		t.Fatal("missing unique backlog query")
	}
	query := strings.Split(parts[1], "\n-- name:")[0]
	query = strings.NewReplacer("sqlc.arg(maximum)", "$1", "sqlc.arg(subject)", "$2").Replace(query)
	if strings.Contains(query, "sqlc.") {
		t.Fatal("unbound diagnostic query argument")
	}
	var plan string
	must(t, f.a.QueryRow(f.ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+query, int64(10000), "active").Scan(&plan))
	if !strings.Contains(plan, "judge_tasks_unfinished") {
		t.Fatalf("active query did not use partial index: %s", plan)
	}
	t.Log("actual backlog plan with 2048 terminal/3 active rows:", plan)
}

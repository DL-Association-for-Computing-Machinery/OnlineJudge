//go:build integration

package integration_test

import (
	"context"
	"errors"
	"math"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const restoredSubject, restoredScope = "restored-lease-owner", "m0-training"

var restoreAC = submission.Result{Verdict: "AC", EvidenceSHA256: materialHash([]byte("fixture AC"))}
var restoreSE = submission.Result{Verdict: "SE", EvidenceSHA256: materialHash([]byte("fixture SE"))}

func TestFenceRestoredLeasesFromArchive(t *testing.T) {
	f := newBacklogFixture(t)
	node := restoreLeaseNode(t, f)
	history := acceptRestoreLease(t, f, "lease-restore-history")
	completed := claimRestoreLease(t, f.ctx, f.first, node, history)
	must(t, f.first.Report(f.ctx, node, completed.Token, restoreAC))
	failed := acceptRestoreLease(t, f, "lease-restore-failed-1")
	for range 3 {
		attempt := claimRestoreLease(t, f.ctx, f.first, node, failed)
		must(t, f.first.Report(f.ctx, node, attempt.Token, restoreSE))
	}
	running := acceptRestoreLease(t, f, "lease-restore-running")
	_, err := f.a.Exec(f.ctx, `UPDATE judge_tasks SET recovery_generation=2 WHERE submission_id=$1`, running)
	must(t, err)
	for range 2 {
		attempt := claimRestoreLease(t, f.ctx, f.first, node, running)
		must(t, f.first.Report(f.ctx, node, attempt.Token, restoreSE))
	}
	old := claimRestoreLease(t, f.ctx, f.first, node, running)
	if old.AttemptNumber != 3 {
		t.Fatal("archive must contain a third in-flight attempt")
	}
	_, err = f.a.Exec(f.ctx, `UPDATE worker_nodes SET capacity=3 WHERE id=$1`, node.ID)
	must(t, err)
	budget := acceptRestoreLease(t, f, "lease-restore-budget-1")
	otherOld := claimRestoreLease(t, f.ctx, f.first, node, budget)
	queued := acceptRestoreLease(t, f, "lease-restore-queued-1")
	queuedAttempt := claimRestoreLease(t, f.ctx, f.first, node, queued)
	must(t, f.first.Report(f.ctx, node, queuedAttempt.Token, restoreSE))
	_, err = f.a.Exec(f.ctx, `UPDATE worker_nodes SET capacity=2 WHERE id=$1`, node.ID)
	must(t, err)
	// An unmatched lease from a superseded generation still takes node capacity.
	stale := strings.Repeat("a", 64)
	_, err = f.a.Exec(f.ctx, `INSERT INTO judge_attempts(token,submission_id,judge_generation,recovery_generation,attempt_number,node_id,credential_version,lease_until)
		VALUES ($1,$2,1,1,4,$3,1,clock_timestamp()+interval '10 minutes')`, stale, running, node.ID)
	must(t, err)
	archive, schema := dumpSubmissionFixture(t, f, t.TempDir())
	target := newSubmissionRestoreTarget(t, f.ctx, schema, archive)
	restored, err := postgres.New(target, experiment.Version, experiment.Manifest())
	must(t, err)
	// The ordinary runtime DSN must never be used as a fallback by this command.
	output, err := runRestoreFenceCommand(t, f.ctx, target, schema, "OJ_DATABASE_URL")
	if err == nil || output != "restore fencing refused: OJ_RESTORED_DATABASE_URL is required" {
		t.Fatalf("restore command accepted ordinary runtime configuration: %q (%v)", output, err)
	}
	// Negative control: the live archived token would work on an unprepared
	// target. Fence and test promptly rather than relying on wall-clock expiry.
	until, err := restored.Renew(f.ctx, node, old.Token)
	must(t, err)
	var rank, order int64
	must(t, target.QueryRow(f.ctx, `SELECT virtual_finish, enqueue_order FROM judge_tasks WHERE submission_id=$1`, running).Scan(&rank, &order))
	for _, expected := range []string{"restored leases expired=3 tasks requeued=2", "restored leases expired=0 tasks requeued=0"} {
		output, err := runRestoreFenceCommand(t, f.ctx, target, schema, "OJ_RESTORED_DATABASE_URL")
		if err != nil || output != expected {
			t.Fatalf("restore command returned unexpected result: %q (%v)", output, err)
		}
	}
	var stillLive bool
	must(t, target.QueryRow(f.ctx, `SELECT clock_timestamp() < $1::timestamptz`, until).Scan(&stillLive))
	if !stillLive {
		t.Fatal("old lease elapsed before the rejection assertions")
	}
	if _, err := restored.Renew(f.ctx, node, old.Token); !errors.Is(err, submission.ErrLease) {
		t.Fatalf("old renewal: %v", err)
	}
	if _, err := restored.Materials(f.ctx, node, old.Token); !errors.Is(err, submission.ErrLease) {
		t.Fatalf("old material read: %v", err)
	}
	if err := restored.Report(f.ctx, node, old.Token, restoreAC); !errors.Is(err, submission.ErrLease) {
		t.Fatalf("late result: %v", err)
	}
	var expired int
	must(t, target.QueryRow(f.ctx, `SELECT count(*) FROM judge_attempts WHERE token IN ($1,$2,$3) AND state='expired' AND result_payload IS NULL`, old.Token, stale, otherOld.Token).Scan(&expired))
	if expired != 3 {
		t.Fatal("archived lease history was not retained as expired")
	}
	var gotRank, gotOrder int64
	must(t, target.QueryRow(f.ctx, `SELECT virtual_finish,enqueue_order FROM judge_tasks WHERE submission_id=$1`, running).Scan(&gotRank, &gotOrder))
	if gotRank != rank || gotOrder != order {
		t.Fatal("recovery changed the original queue position")
	}
	for _, item := range []struct{ id, state string }{{history, "finished"}, {failed, "failed"}, {queued, "queued"}} {
		var state string
		var generation int64
		must(t, target.QueryRow(f.ctx, `SELECT state,recovery_generation FROM judge_tasks WHERE submission_id=$1`, item.id).Scan(&state, &generation))
		if state != item.state || generation != 1 {
			t.Fatal("recovery touched a task that was not running")
		}
	}
	// An exact acknowledgement for an already completed archived attempt is read-only.
	must(t, restored.Report(f.ctx, node, completed.Token, restoreAC))
	replacement := claimRestoreLease(t, f.ctx, restored, node, running)
	if replacement.Token == old.Token || replacement.JudgeGeneration != old.JudgeGeneration || replacement.RecoveryGeneration != old.RecoveryGeneration+1 || replacement.AttemptNumber != 1 {
		t.Fatal("replacement did not start at attempt 1 under a new recovery generation")
	}
	material, err := restored.Materials(f.ctx, node, replacement.Token)
	must(t, err)
	if material.SubmissionID != running || material.Source != f.request.Source {
		t.Fatal("replacement lost the accepted source")
	}
	must(t, restored.Report(f.ctx, node, replacement.Token, restoreAC))
	if err := restored.Report(f.ctx, node, old.Token, restoreSE); !errors.Is(err, submission.ErrLease) {
		t.Fatalf("late result after replacement completed: %v", err)
	}
	got, err := restored.Get(f.ctx, restoredSubject, restoredScope, running)
	must(t, err)
	if got.State != "finished" || got.Verdict == nil || *got.Verdict != "AC" {
		t.Fatal("replacement result did not remain authoritative")
	}
	for n := int32(1); n <= 3; n++ {
		fresh := claimRestoreLease(t, f.ctx, restored, node, budget)
		if fresh.RecoveryGeneration != otherOld.RecoveryGeneration+1 || fresh.AttemptNumber != n {
			t.Fatal("claim did not use the restored task's new recovery generation and bounded attempt sequence")
		}
		must(t, restored.Report(f.ctx, node, fresh.Token, restoreSE))
	}
	// Three system failures in the new generation terminate this task. The next
	// existing queued submission still completes with a valid current report.
	got, err = restored.Get(f.ctx, restoredSubject, restoredScope, budget)
	must(t, err)
	if got.State != "failed" || got.Verdict != nil {
		t.Fatal("recovered generation exceeded its attempt budget")
	}
	fresh := claimRestoreLease(t, f.ctx, restored, node, queued)
	if fresh.RecoveryGeneration != queuedAttempt.RecoveryGeneration || fresh.AttemptNumber != queuedAttempt.AttemptNumber+1 {
		t.Fatal("already queued task lost its existing attempt budget")
	}
	must(t, restored.Report(f.ctx, node, fresh.Token, restoreAC))
	t.Log("restored live lease fenced before expiry; replacement reclaimed original source and queue position with three attempts in its new generation")
}

func TestFenceRestoredLeasesRollsBack(t *testing.T) {
	for _, name := range []string{"wrong database", "contention", "missing current lease", "generation overflow"} {
		t.Run(name, func(t *testing.T) {
			f := newBacklogFixture(t)
			node := restoreLeaseNode(t, f)
			first := claimRestoreLease(t, f.ctx, f.first, node, acceptRestoreLease(t, f, "lease-rollback-first"))
			var dbName, code string
			must(t, f.a.QueryRow(f.ctx, `SELECT current_database()`).Scan(&dbName))
			var expected error
			switch name {
			case "wrong database":
				dbName += "-not-the-connected-database"
				expected = postgres.ErrRestoreDatabase
			case "contention":
				tx, err := f.b.Begin(f.ctx)
				must(t, err)
				t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
				_, err = tx.Exec(f.ctx, `LOCK TABLE judge_attempts IN ACCESS SHARE MODE`)
				must(t, err)
				code = "55P03"
			case "missing current lease", "generation overflow":
				_, err := f.a.Exec(f.ctx, `UPDATE worker_nodes SET capacity=2 WHERE id=$1`, node.ID)
				must(t, err)
				second := claimRestoreLease(t, f.ctx, f.first, node, acceptRestoreLease(t, f, "lease-rollback-second"))
				if name == "missing current lease" {
					_, err = f.a.Exec(f.ctx, `UPDATE judge_attempts SET state='expired' WHERE token=$1`, second.Token)
					expected = postgres.ErrRestoreState
				} else {
					_, err = f.a.Exec(f.ctx, `UPDATE judge_tasks SET recovery_generation=$2 WHERE submission_id=$1`, second.SubmissionID, int64(math.MaxInt64))
					must(t, err)
					_, err = f.a.Exec(f.ctx, `UPDATE judge_attempts SET recovery_generation=$2 WHERE token=$1`, second.Token, int64(math.MaxInt64))
					code = "22003"
				}
				must(t, err)
			}
			changes, err := postgres.FenceRestoredLeases(f.ctx, f.a, dbName)
			var pgErr *pgconn.PgError
			if changes != (postgres.RestoredLeaseChanges{}) || (expected != nil && !errors.Is(err, expected)) ||
				(code != "" && (!errors.As(err, &pgErr) || pgErr.Code != code)) {
				t.Fatalf("unexpected fencing result: %+v %v", changes, err)
			}
			var task, attempt string
			var generation int64
			must(t, f.a.QueryRow(f.ctx, `SELECT t.state,a.state,t.recovery_generation FROM judge_tasks t JOIN judge_attempts a USING(submission_id) WHERE a.token=$1`, first.Token).Scan(&task, &attempt, &generation))
			if task != "running" || attempt != "leased" || generation != first.RecoveryGeneration {
				t.Fatal("failed fencing changed unrelated live work")
			}
		})
	}
}

func restoreLeaseNode(t *testing.T, f backlogFixture) submission.NodeIdentity {
	t.Helper()
	backlogPolicy(t, f, 8, 8)
	node := submission.NodeIdentity{ID: "archive-lease-node", CredentialVersion: 1}
	_, err := f.a.Exec(f.ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)`, node.ID)
	must(t, err)
	return node
}

func acceptRestoreLease(t *testing.T, f backlogFixture, key string) string {
	t.Helper()
	r, created, err := f.first.Accept(f.ctx, restoredSubject, restoredScope, key, f.request)
	must(t, err)
	if !created {
		t.Fatal("expected a new archive fixture submission")
	}
	return r.ID
}

func claimRestoreLease(t *testing.T, ctx context.Context, store *postgres.Store, node submission.NodeIdentity, id string) submission.Lease {
	t.Helper()
	lease, err := store.Claim(ctx, node)
	must(t, err)
	if lease.SubmissionID != id {
		t.Fatal("claimed the wrong submission")
	}
	return lease
}

func runRestoreFenceCommand(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema, dsnVariable string) (string, error) {
	t.Helper()
	path, err := filepath.Abs("../../.local/bin/migrate")
	must(t, err)
	config := pool.Config()
	cmd := exec.CommandContext(ctx, path, "fence-restored-leases", config.ConnConfig.Database)
	cmd.Env = []string{dsnVariable + "=" + config.ConnString() + " search_path=" + schema}
	raw, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(raw)), err
}

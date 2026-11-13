//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

func TestTrainingQueueMigration(t *testing.T) {
	f := newBacklogFixture(t)
	_, err := f.provider.DownTo(f.ctx, 19)
	must(t, err)
	if db.CheckSchema(f.ctx, f.a) == nil {
		t.Fatal("current API accepted schema 19")
	}
	backlogPolicy(t, f, 100, 100)
	_, err = f.a.Exec(f.ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES('migration-train',1);
		INSERT INTO submissions (id,subject_id,context_id,idempotency_key,request_sha256,material_version,manifest_sha256,language,source_code,created_at)
		SELECT id,'migrated-user',scope,'migration-key-'||id,repeat('c',64),version_id,manifest_sha256,'gnu++17','// synthetic',
			'2026-01-01 00:00:00+00'::timestamptz + offset_s * interval '1 second'
		FROM submission_materials CROSS JOIN (VALUES
			('old-terminal','left',0),('old-queued','left',1),('old-running','right',2),('old-retry','left',3),('old-pending','right',4)) f(id,scope,offset_s);
		INSERT INTO judge_tasks(submission_id,state,attempt_number,verdict) VALUES
			('old-terminal','finished',1,'AC'),('old-queued','queued',0,NULL),('old-running','running',1,NULL),
			('old-retry','queued',1,NULL),('old-pending','queued',0,NULL);
		INSERT INTO judge_attempts(token,submission_id,judge_generation,recovery_generation,attempt_number,node_id,credential_version,lease_until)
		VALUES ('old-token','old-running',1,1,1,'migration-train',1,clock_timestamp()+interval '1 minute')`)
	must(t, err)
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
	rows, err := f.a.Query(f.ctx, `SELECT submission_id,virtual_finish,enqueue_order FROM judge_tasks WHERE queue_version=$1 ORDER BY enqueue_order`, f.request.MaterialVersion)
	must(t, err)
	var got []string
	for rows.Next() {
		var id string
		var rank, order int
		must(t, rows.Scan(&id, &rank, &order))
		got = append(got, id)
		if rank != len(got) || order != len(got) {
			t.Fatalf("unexpected backfill for %s: (%d,%d)", id, rank, order)
		}
	}
	rows.Close()
	must(t, rows.Err())
	if strings.Join(got, ",") != "old-queued,old-running,old-retry,old-pending" {
		t.Fatalf("unexpected migrated queue: %v", got)
	}
	newTask, _, err := f.first.Accept(f.ctx, "new-user", "right", "migrated-new-001", f.request)
	must(t, err)
	var floor, rank, order int
	must(t, f.a.QueryRow(f.ctx, `SELECT c.floor,t.virtual_finish,t.enqueue_order FROM judge_tasks t
		JOIN training_queue_clock c ON c.version_id=t.queue_version WHERE t.submission_id=$1`, newTask.ID).Scan(&floor, &rank, &order))
	if floor != 3 || rank != 4 || order != 5 {
		t.Fatalf("new task did not follow migrated clock and order: (%d,%d,%d)", floor, rank, order)
	}
	if _, err = f.provider.DownTo(f.ctx, 19); err == nil || !strings.Contains(err.Error(), "cannot discard active training queue order") {
		t.Fatalf("unexpected rollback result with active tasks: %v", err)
	}
	node := submission.NodeIdentity{ID: "migration-train", CredentialVersion: 1}
	ac := submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("a", 64)}
	must(t, f.first.Report(f.ctx, node, "old-token", ac))
	for _, id := range []string{"old-queued", "old-retry", "old-pending", newTask.ID} {
		lease, err := f.first.Claim(f.ctx, node)
		must(t, err)
		if lease.SubmissionID != id {
			t.Fatalf("migrated claim got %s, want %s", lease.SubmissionID, id)
		}
		must(t, f.first.Report(f.ctx, node, lease.Token, ac))
	}
	_, err = f.provider.DownTo(f.ctx, 19)
	must(t, err)
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	var history int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM judge_tasks WHERE state='finished' AND verdict='AC' AND queue_version IS NULL`).Scan(&history))
	if history != 6 {
		t.Fatalf("rollback/re-up changed terminal history: %d", history)
	}
}

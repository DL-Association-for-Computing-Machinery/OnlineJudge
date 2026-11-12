//go:build integration

package integration_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func TestTrainingQueueQueryPlans(t *testing.T) {
	f := newBacklogFixture(t)
	const otherVersion = "plan-foreign-training"
	other, err := postgres.New(f.a, otherVersion, []byte(`{"training":"foreign"}`))
	must(t, err)
	must(t, other.RegisterFixture(f.ctx))
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(f.ctx, `INSERT INTO submissions
		(id,subject_id,context_id,idempotency_key,request_sha256,material_version,manifest_sha256,language,source_code)
		SELECT 'plan-'||n, CASE WHEN n%2=0 THEN 'selected' ELSE 'other' END,'training','training-plan-key-'||n,repeat('a',64),
		       m.version_id,m.manifest_sha256,'gnu++17','// synthetic'
		FROM generate_series(1,12296) n JOIN submission_materials m ON m.version_id=CASE WHEN n BETWEEN 8193 AND 12288 THEN $1 ELSE $2 END`,
		otherVersion, f.request.MaterialVersion)
	must(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO judge_tasks(submission_id,state,verdict) SELECT 'plan-'||n,'finished','AC' FROM generate_series(1,8192) n;
		INSERT INTO judge_tasks(submission_id,queue_version,virtual_finish,enqueue_order)
		SELECT id,material_version,n,n FROM generate_series(8193,12296) n JOIN submissions ON id='plan-'||n;
		ANALYZE judge_tasks; ANALYZE submissions`)
	must(t, err)
	for _, tc := range []struct {
		file, name string
		args       []any
	}{
		{"leases.sql", "ClaimCandidate", []any{f.request.MaterialVersion}},
		{"submissions.sql", "MaxQueuedTrainingRank", []any{f.request.MaterialVersion, "selected"}},
	} {
		raw, err := os.ReadFile("../../db/queries/" + tc.file)
		must(t, err)
		_, rest, found := strings.Cut(string(raw), "-- name: "+tc.name+" :one\n")
		if !found {
			t.Fatal("query not found:", tc.name)
		}
		query, _, _ := strings.Cut(rest, ";")
		query = strings.NewReplacer("sqlc.arg(version_id)", "$1", "sqlc.arg(subject_id)", "$2").Replace(query)
		rows, err := tx.Query(f.ctx, "EXPLAIN (ANALYZE, BUFFERS, COSTS OFF) "+query, tc.args...)
		must(t, err)
		var plan strings.Builder
		for rows.Next() {
			var line string
			must(t, rows.Scan(&line))
			plan.WriteString(line + "\n")
		}
		rows.Close()
		must(t, rows.Err())
		result := plan.String()
		if !strings.Contains(result, "judge_tasks_training_queue") || !strings.Contains(result, "Index Cond: (queue_version =") || strings.Contains(result, "Sort") {
			t.Fatalf("%s did not use the ordered, version-scoped index:\n%s", tc.name, result)
		}
		t.Logf("%s with 8192 terminal, 4096 foreign and 8 matching queued tasks:\n%s", tc.name, result)
	}
}

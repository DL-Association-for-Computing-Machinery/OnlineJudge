//go:build integration

package integration_test

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

func TestSubmissionHistoryPagingAndRollback(t *testing.T) {
	f := newBacklogFixture(t)
	backlogPolicy(t, f, 100, 100)
	const owner, scope = "training-owner", "training"
	accept := func(subject, contextID string, seq int) submission.Receipt {
		t.Helper()
		r, created, err := f.first.Accept(f.ctx, subject, contextID, fmt.Sprintf("history-request-%04d", seq), f.request)
		must(t, err)
		if !created {
			t.Fatal("expected new submission")
		}
		return r
	}
	ids := make([]string, 22)
	for n := range ids {
		ids[n] = accept(owner, scope, n).ID
	}
	other := accept("other-owner", scope, 0)
	otherScope := accept(owner, "different-context", 0)
	_, err := f.a.Exec(f.ctx, "UPDATE submissions SET created_at = '2026-11-13T01:02:03.123456Z'")
	must(t, err)
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	_, err = f.a.Exec(f.ctx, "UPDATE judge_tasks SET state='finished', verdict='AC' WHERE submission_id=$1", ids[0])
	must(t, err)
	first, err := f.second.ListHistory(f.ctx, owner, scope, nil)
	must(t, err)
	if len(first.Items) != 20 || first.Next == nil || first.Next.ID != ids[19] || first.Items[0].Verdict == nil || *first.Items[0].Verdict != "AC" {
		t.Fatalf("unexpected first page: %+v", first)
	}
	newer := accept(owner, scope, 22)
	// Use a fixed clock for both sides of the cursor in this fixture.
	_, err = f.a.Exec(f.ctx, "UPDATE submissions SET created_at = '2026-11-13T01:02:04Z' WHERE id=$1", newer.ID)
	must(t, err)
	second, err := f.second.ListHistory(f.ctx, owner, scope, first.Next)
	must(t, err)
	if len(second.Items) != 2 || second.Next != nil {
		t.Fatalf("unexpected last page: %+v", second)
	}
	for i, item := range append(first.Items, second.Items...) {
		if item.ID != ids[i] || item.Language != "gnu++17" || item.MaterialVersion != f.request.MaterialVersion {
			t.Fatal("missing, foreign or out-of-order summary")
		}
	}
	for _, test := range []struct{ subject, scope, id string }{{"other-owner", scope, other.ID}, {owner, "different-context", otherScope.ID}, {owner, "unused", ""}} {
		page, err := f.first.ListHistory(f.ctx, test.subject, test.scope, nil)
		must(t, err)
		if page.Next != nil || page.Items == nil || (test.id == "" && len(page.Items) != 0) ||
			(test.id != "" && (len(page.Items) != 1 || page.Items[0].ID != test.id)) {
			t.Fatal("history escaped owner or scope")
		}
	}
	if _, err := f.first.ListHistory(f.ctx, owner, scope, &submission.HistoryPosition{CreatedAt: first.Next.CreatedAt, ID: "invalid"}); !errors.Is(err, submission.ErrInvalid) {
		t.Fatal("invalid cursor accepted")
	}
	checkHistoryPlan(t, f, owner, scope, *first.Next)
	_, err = f.provider.DownTo(f.ctx, 22)
	must(t, err)
	backlogCounts(t, f, 25)
	if db.CheckSchema(f.ctx, f.a) == nil {
		t.Fatal("new binary accepted the older schema")
	}
	var indexMissing bool
	must(t, f.a.QueryRow(f.ctx, "SELECT to_regclass('submissions_owner_history') IS NULL").Scan(&indexMissing))
	if !indexMissing {
		t.Fatal("rollback retained the history index")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
	again, err := f.first.Get(f.ctx, owner, scope, ids[0])
	must(t, err)
	if again.Verdict == nil || *again.Verdict != "AC" {
		t.Fatal("rollback changed an accepted result")
	}
}

func checkHistoryPlan(t *testing.T, f backlogFixture, owner, scope string, after submission.HistoryPosition) {
	t.Helper()
	// Give the planner unrelated rows so the indexed seek has a meaningful
	// alternative. Fixture-only copies are rolled back after plan inspection.
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `INSERT INTO submissions(id,subject_id,context_id,idempotency_key,request_sha256,material_version,manifest_sha256,language,source_code)
		SELECT md5('history-padding-'||n), 'unrelated-'||n, s.context_id, s.idempotency_key, s.request_sha256,
		    s.material_version,s.manifest_sha256,s.language,s.source_code
		FROM (SELECT * FROM submissions LIMIT 1) s CROSS JOIN generate_series(1,4096) n`)
	must(t, err)
	_, err = tx.Exec(f.ctx, "ANALYZE submissions")
	must(t, err)
	rows, err := tx.Query(f.ctx, `EXPLAIN (ANALYZE, BUFFERS)
		SELECT s.id,s.material_version,s.language,s.created_at,t.state,t.verdict
		FROM submissions s JOIN judge_tasks t ON t.submission_id=s.id
		WHERE s.subject_id=$1 AND s.context_id=$2 AND (s.created_at,s.id)<($3,$4)
		ORDER BY s.created_at DESC,s.id DESC LIMIT 21`, owner, scope, after.CreatedAt, after.ID)
	must(t, err)
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		must(t, rows.Scan(&line))
		plan.WriteString(line + "\n")
	}
	must(t, rows.Err())
	if !strings.Contains(plan.String(), "Index Scan using submissions_owner_history") {
		t.Fatalf("expected an index seek:\n%s", plan.String())
	}
	t.Log(plan.String())
}

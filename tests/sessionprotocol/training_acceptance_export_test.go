//go:build integration

package sessionprotocol_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func testTrainingAcceptanceExport(t *testing.T, f *fixture, version string) {
	t.Helper()
	var database string
	var from time.Time
	must(t, f.pools[0].QueryRow(f.ctx, `SELECT current_database(), date_trunc('second', min(created_at))-interval '1 minute' FROM submissions`).Scan(&database, &from))
	until := from.Add(30 * time.Second)
	rows, err := f.pools[0].Query(f.ctx, `SELECT id FROM submissions ORDER BY id`)
	must(t, err)
	var ids []string
	for rows.Next() {
		var id string
		must(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	rows.Close()
	must(t, rows.Err())
	if len(ids) != 12 {
		t.Fatal("expected four durable receipts per overload round")
	}
	// Set explicit boundaries and a cross-context control on the disposable DB.
	_, err = f.pools[0].Exec(f.ctx, `UPDATE submissions SET created_at=$1 WHERE id=$2 OR id=$3`, from, ids[0], ids[1])
	must(t, err)
	_, err = f.pools[0].Exec(f.ctx, `UPDATE submissions SET context_id='not-personal-training' WHERE id=$1`, ids[1])
	must(t, err)
	_, err = f.pools[0].Exec(f.ctx, `UPDATE submissions SET created_at=$1 WHERE id=$2`, until, ids[2])
	must(t, err)
	_, err = f.pools[0].Exec(f.ctx, `CREATE ROLE acceptance_observer;
		GRANT SELECT (id, version_id) ON goose_db_version TO acceptance_observer;
		GRANT SELECT (id, created_at, material_version, context_id) ON submissions TO acceptance_observer`)
	must(t, err)
	binary := entryProgram(t, "migrate")
	run := func(name string, start, end time.Time) ([]byte, error) {
		cmd := exec.CommandContext(t.Context(), binary, "training-acceptance", name, start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano))
		cmd.Env = []string{"OJ_JUDGE_SNAPSHOT_DATABASE_URL=" + os.Getenv("OJ_PROTOCOL_DATABASE_URL") +
			" options='-c role=acceptance_observer -c default_transaction_read_only=on'"}
		return cmd.Output()
	}
	raw, err := run(database, from, until)
	must(t, err)
	var observation submissionpg.TrainingAcceptanceObservation
	must(t, json.Unmarshal(raw, &observation))
	if observation.Schema != "oj.p6-training-acceptance.v1" || !observation.From.Equal(from) ||
		!observation.Until.Equal(until) || observation.ObservedAt.Before(until) || len(observation.Accepted) != 1 ||
		observation.Accepted[0].ID != ids[0] || !observation.Accepted[0].AcceptedAt.Equal(from) ||
		observation.Accepted[0].MaterialVersion != version {
		t.Fatal("acceptance export did not honor the closed half-open window and training scope")
	}
	var fields map[string]json.RawMessage
	must(t, json.Unmarshal(raw, &fields))
	var receipts []map[string]json.RawMessage
	must(t, json.Unmarshal(fields["accepted"], &receipts))
	if len(fields) != 5 || len(receipts[0]) != 3 || strings.Contains(string(raw), "synthetic-p6-overload-") ||
		strings.Contains(string(raw), "int main()") {
		t.Fatal("acceptance export contains fields outside the receipt observation")
	}
	broader := time.Now().UTC()
	raw, err = run(database, from, broader)
	must(t, err)
	must(t, json.Unmarshal(raw, &observation))
	got := make([]string, 0, len(observation.Accepted))
	for _, r := range observation.Accepted {
		got = append(got, r.ID)
	}
	slices.Sort(got)
	want := append([]string{ids[0]}, ids[2:]...)
	if !slices.Equal(got, want) {
		t.Fatal("database-led export lost or duplicated durable receipts")
	}
	for _, negative := range []struct {
		db  string
		end time.Time
	}{{database + "-wrong", until}, {database, broader.Add(time.Minute)}} {
		if out, err := run(negative.db, from, negative.end); err == nil || len(out) != 0 {
			t.Fatal("unsafe acceptance window returned output")
		}
	}
	var rawPlan string
	must(t, f.pools[1].QueryRow(f.ctx, `EXPLAIN (ANALYZE, FORMAT JSON) SELECT id, created_at, material_version FROM submissions
		WHERE context_id=$1 AND created_at >= $2 AND created_at < $3 ORDER BY created_at, id LIMIT $4`,
		submission.PersonalTrainingScope, from, broader, 20_001).Scan(&rawPlan))
	var plans []struct {
		Plan struct {
			Rows int `json:"Actual Rows"`
		}
		ExecutionMS float64 `json:"Execution Time"`
	}
	must(t, json.Unmarshal([]byte(rawPlan), &plans))
	if len(plans) != 1 || plans[0].Plan.Rows != len(want) {
		t.Fatal("acceptance query plan did not read the expected rows")
	}
	t.Logf("closed-window export passed with column-level SELECT only; PostgreSQL EXPLAIN ANALYZE: rows=%d execution_ms=%.3f",
		plans[0].Plan.Rows, plans[0].ExecutionMS)
}

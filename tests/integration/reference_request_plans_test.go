//go:build integration

package integration_test

import (
	"os"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
)

func TestReferenceRequestMigrationSafety(t *testing.T) {
	for _, mode := range []string{"empty upgrade and rollback", "configured", "queued", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			f := newReferenceRequestFixture(t)
			if mode == "empty upgrade and rollback" {
				_, err := f.provider.DownTo(f.ctx, 8)
				must(t, err)
				if db.CheckSchema(f.ctx, f.a) == nil {
					t.Fatal("schema gate accepted old version 8")
				}
				_, err = f.provider.Up(f.ctx)
				must(t, err)
				must(t, db.CheckSchema(f.ctx, f.a))
				unchanged, err := f.repository.GetOwn(f.ctx, f.actor, f.draft.ID)
				must(t, err)
				if unchanged.Version != f.draft.Version || unchanged.ValidationState != "not-run" {
					t.Fatal("schema 9 upgrade changed existing draft")
				}
				return
			}
			f.limits(t, 1, 1)
			if mode != "configured" {
				_, err := f.store.Enqueue(f.ctx, f.actor, "reference-migration-intent", f.draft.ID, f.catalog)
				must(t, err)
				if mode == "cancelled" {
					_, err = f.store.Cancel(f.ctx, f.actor, "reference-migration-intent")
					must(t, err)
				}
			}
			if _, err := f.provider.DownTo(f.ctx, 8); err == nil {
				t.Fatal("rollback discarded configured admission or request history")
			}
			_, err := f.provider.Up(f.ctx)
			must(t, err)
			must(t, db.CheckSchema(f.ctx, f.a))
			total, queued := 1, 1
			if mode == "configured" {
				total, queued = 0, 0
			} else if mode == "cancelled" {
				queued = 0
			}
			f.counts(t, total, queued)
		})
	}
}

func TestReferenceRequestQueryPlans(t *testing.T) {
	f := newReferenceRequestFixture(t)
	f.limits(t, 32, 2)
	_, err := f.store.Enqueue(f.ctx, f.actor, "reference-plan-original-key", f.draft.ID, f.catalog)
	must(t, err)
	// Terminal history dominates; only 30 queued rows belong in the partial index.
	_, err = f.a.Exec(f.ctx, `INSERT INTO problem_reference_requests
        (id,owner_subject_id,request_key,draft_id,source_id,content_version,import_revision,source_lock_sha256,identity_protocol,profile_encoding,profile_sha256,auth_generation,recovery_id,author_revision,state,cancelled_at)
        SELECT md5('reference-plan-'||n::text),r.owner_subject_id,'reference-plan-key-'||n::text,r.draft_id,r.source_id,r.content_version,r.import_revision,r.source_lock_sha256,r.identity_protocol,r.profile_encoding,r.profile_sha256,r.auth_generation,r.recovery_id,r.author_revision,
        CASE WHEN n<=29 THEN 'queued' ELSE 'cancelled' END,CASE WHEN n<=29 THEN NULL ELSE clock_timestamp() END
        FROM problem_reference_requests r CROSS JOIN generate_series(1,10000) n WHERE r.request_key='reference-plan-original-key'`)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `ANALYZE problem_reference_requests`)
	must(t, err)
	raw, err := os.ReadFile("../../db/queries/problemreference/requests.sql")
	must(t, err)
	for _, tc := range []struct {
		name, index  string
		args         []any
		replacements []string
	}{
		{"ReferenceRequest", "problem_reference_requests_owner_subject_id_request_key_key", []any{f.actor.SubjectID, "reference-plan-key-5000"}, []string{"@owner", "$1", "@request_key", "$2"}},
		{"ReferenceRequestForUpdate", "problem_reference_requests_owner_subject_id_request_key_key", []any{f.actor.SubjectID, "reference-plan-key-5000"}, []string{"@owner", "$1", "@request_key", "$2"}},
		{"ReferenceQueuedCounts", "problem_reference_requests_queued_owner", []any{int64(32), f.actor.SubjectID}, []string{"sqlc.arg(maximum)", "$1", "sqlc.arg(owner)", "$2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := strings.Split(string(raw), "-- name: "+tc.name+" :")
			if len(parts) != 2 {
				t.Fatal("missing authoritative request query")
			}
			query := strings.SplitN(parts[1], "\n", 2)[1]
			query = strings.Split(query, "\n-- name:")[0]
			query = strings.NewReplacer(tc.replacements...).Replace(query)
			var plan string
			must(t, f.a.QueryRow(f.ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+query, tc.args...).Scan(&plan))
			if !strings.Contains(plan, tc.index) {
				t.Fatalf("query lost its bounded index: %s", plan)
			}
			if tc.name == "ReferenceQueuedCounts" && !strings.Contains(plan, `"Node Type": "Limit"`) {
				t.Fatal("queue count lost its hard window")
			}
			t.Log(plan)
		})
	}
	var indexBytes int64
	must(t, f.a.QueryRow(f.ctx, `SELECT pg_relation_size('problem_reference_requests_queued_owner')`).Scan(&indexBytes))
	t.Logf("synthetic 10001 rows/30 queued partial index bytes=%d; not production capacity", indexBytes)
}

func TestReferenceRequestLoweredLimitsAndCorruptReceipt(t *testing.T) {
	f := newReferenceRequestFixture(t)
	f.limits(t, 2, 2)
	for _, key := range []string{"reference-lowered-first", "reference-lowered-second"} {
		_, err := f.store.Enqueue(f.ctx, f.actor, key, f.draft.ID, f.catalog)
		must(t, err)
	}
	must(t, referencepg.Configure(f.ctx, f.a, problemreference.Limits{Global: 1, Owner: 1}))
	r, err := f.store.Enqueue(f.ctx, f.actor, "reference-lowered-third", f.draft.ID, f.catalog)
	noReferenceResult(t, r, err, problemreference.ErrFull)
	_, err = f.store.Receipt(f.ctx, f.actor, "reference-lowered-first")
	must(t, err)
	f.counts(t, 2, 2)
	_, err = f.a.Exec(f.ctx, `UPDATE problem_reference_requests SET profile_encoding='{}'::bytea WHERE request_key=$1`, "reference-lowered-first")
	must(t, err)
	r, err = f.store.Receipt(f.ctx, f.actor, "reference-lowered-first")
	noReferenceResult(t, r, err, problemreference.ErrStore)
}

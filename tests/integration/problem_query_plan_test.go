//go:build integration

package integration_test

import (
	"os"
	"strings"
	"testing"
)

func TestPrivateDraftQueryPlans(t *testing.T) {
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	content := f.prepared.Content()
	_, err := f.a.Exec(f.ctx, `INSERT INTO problem_drafts(id,owner_subject_id,request_key,request_sha256,source_id,content_version,document)
SELECT md5('draft-plan-'||n::text),$1,'private-plan-key-'||n::text,$2,$3,$4,$5 FROM generate_series(1,2048) n`, f.actor.SubjectID, content.RequestSHA256, content.SourceID, content.Version, content.Document)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `INSERT INTO problem_draft_materials(draft_id,logical_name,sha256,byte_size) SELECT id,'statement',$1,18 FROM problem_drafts`, strings.Repeat("a", 64))
	must(t, err)
	_, err = f.a.Exec(f.ctx, `ANALYZE problem_drafts; ANALYZE problem_draft_materials`)
	must(t, err)
	raw, err := os.ReadFile("../../db/queries/problem/drafts.sql")
	must(t, err)
	for _, tc := range []struct {
		name, index string
		args        []any
		replace     []string
	}{
		{"DraftByKeyForShare", "problem_drafts_owner_subject_id_request_key_key", []any{f.actor.SubjectID, "private-plan-key-1024"}, []string{"@owner", "$1", "@request_key", "$2"}},
		{"DraftByOwner", "problem_drafts_pkey", []any{"", f.actor.SubjectID}, []string{"@id", "$1", "@owner", "$2"}},
		{"DraftMaterials", "problem_draft_materials_pkey", []any{""}, []string{"@draft_id", "$1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := strings.Split(string(raw), "-- name: "+tc.name+" :")
			if len(parts) != 2 {
				t.Fatal("missing unique authoritative draft query")
			}
			query := strings.SplitN(parts[1], "\n", 2)[1]
			query = strings.Split(query, "\n-- name:")[0]
			query = strings.NewReplacer(tc.replace...).Replace(query)
			// Choose an actual populated PK, rather than timing an absent-key plan.
			if tc.name != "DraftByKeyForShare" {
					must(t, f.a.QueryRow(f.ctx, `SELECT id FROM problem_drafts WHERE request_key=$1`, "private-plan-key-1024").Scan(&tc.args[0]))
			}
			var plan string
			must(t, f.a.QueryRow(f.ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+query, tc.args...).Scan(&plan))
			if !strings.Contains(plan, tc.index) {
				t.Fatalf("owner/key query did not use its bounded index: %s", plan)
			}
			t.Log(plan)
		})
	}
}

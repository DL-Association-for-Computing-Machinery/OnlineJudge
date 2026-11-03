//go:build integration

package integration_test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
)

func insertListRows(t *testing.T, f draftFixture, owner string, n int) {
	t.Helper()
	_, err := f.a.Exec(f.ctx, `INSERT INTO problem_drafts(id,owner_subject_id,request_key,request_sha256,source_id,content_version,document,created_at)
		SELECT md5($1 || n::text), $1, 'list-fixture-key-' || n::text, repeat('a',64), 'synthetic', repeat('b',64), 'x'::bytea,
		'2026-01-01 00:00:00+00'::timestamptz + (n / 3) * interval '1 microsecond'
		FROM generate_series(1,$2::int) n`, owner, n)
	must(t, err)
}

func TestOwnDraftListPaginationAndMigration(t *testing.T) {
	for _, n := range []int{0, 20, 21, 45} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			f := newDraftFixture(t)
			f.role(t, f.actor.SubjectID, 0, true)
			other := identityID(1101)
			f.role(t, other, 0, true)
			insertListRows(t, f, f.actor.SubjectID, n)
			insertListRows(t, f, other, 100)
			wantRows, err := f.a.Query(f.ctx, `SELECT id FROM problem_drafts WHERE owner_subject_id=$1 ORDER BY created_at DESC,id COLLATE "C" DESC`, f.actor.SubjectID)
			must(t, err)
			var want []string
			for wantRows.Next() {
				var id string
				must(t, wantRows.Scan(&id))
				want = append(want, id)
			}
			must(t, wantRows.Err())
			wantRows.Close()
			var got []string
			var after *problem.ListPosition
			for {
				rows, e := f.repository.ListOwn(f.ctx, f.actor, after)
				must(t, e)
				if len(rows) > 21 {
					t.Fatal("unbounded result")
				}
				limit := len(rows)
				if limit > 20 {
					limit = 20
				}
				for _, row := range rows[:limit] {
					if !row.Valid() {
						t.Fatal("invalid receipt")
					}
					got = append(got, row.DraftID)
				}
				if len(rows) <= 20 {
					break
				}
				last := rows[19]
				after = &problem.ListPosition{CreatedAt: last.CreatedAt, ID: last.DraftID}
			}
			if strings.Join(got, ",") != strings.Join(want, ",") || len(got) != n {
				t.Fatal("pagination missed, duplicated, reordered or crossed owner")
			}
			if n == 21 {
				_, err = f.provider.DownTo(f.ctx, 7)
				must(t, err)
				if db.CheckSchema(f.ctx, f.a) == nil {
					t.Fatal("schema gate accepted version 7")
				}
				var drafts, roles int
				must(t, f.a.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM problem_drafts),(SELECT count(*) FROM identity_problem_authors)`).Scan(&drafts, &roles))
				if drafts != 121 || roles != 2 {
					t.Fatal("index rollback changed data or grants")
				}
				_, err = f.provider.Up(f.ctx)
				must(t, err)
				must(t, db.CheckSchema(f.ctx, f.a))
				rows, err := f.repository.ListOwn(f.ctx, f.actor, nil)
				must(t, err)
				if len(rows) != 21 {
					t.Fatal("migration changed results")
				}
			}
		})
	}
}

func TestOwnDraftListCurrentAuthority(t *testing.T) {
	for _, mode := range []string{"role", "account", "recovery", "lost-transaction"} {
		t.Run(mode, func(t *testing.T) {
			f := newDraftFixture(t)
			f.role(t, f.actor.SubjectID, 0, true)
			insertListRows(t, f, f.actor.SubjectID, 2)
			held := heldDraftAccess{access: f.access, locked: make(chan uint32, 1), release: make(chan struct{}), fixture: f, lose: mode == "lost-transaction"}
			repository, err := problempg.New(f.a, held)
			must(t, err)
			if held.lose {
				rows, e := repository.ListOwn(f.ctx, f.actor, nil)
				if !errors.Is(e, problem.ErrStore) || rows != nil {
					t.Fatal("dead transaction returned rows", e)
				}
				return
			}
			read := make(chan error, 1)
			go func() { _, e := repository.ListOwn(f.ctx, f.actor, nil); read <- e }()
			var pid uint32
			select {
			case pid = <-held.locked:
			case e := <-read:
				t.Fatalf("list failed before locks: %v", e)
			case <-f.ctx.Done():
				t.Fatal("list barrier timed out")
			}
			revoked := make(chan error, 1)
			go func() {
				var e error
				switch mode {
				case "role":
					_, e = f.access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: f.actor.SubjectID, ExpectedRevision: 1, Enabled: false, Operator: "list-fixture", Reason: "concurrent list revocation"})
				case "account":
					_, e = f.identities.DisableCAS(f.ctx, f.actor.SubjectID, identity.Revision{CredentialVersion: 1, AuthGeneration: 1})
				case "recovery":
					_, e = f.identities.BeginRecovery(f.ctx)
				}
				revoked <- e
			}()
			waitBacklogBlocked(t, f.backlogFixture, pid, f.appA)
			close(held.release)
			must(t, <-read)
			must(t, <-revoked)
			rows, e := f.repository.ListOwn(f.ctx, f.actor, nil)
			if e == nil || rows != nil {
				t.Fatal("stale authority returned rows", e)
			}
		})
	}
}

func TestOwnDraftListTieSpansSeveralPages(t *testing.T) {
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	insertListRows(t, f, f.actor.SubjectID, 45)
	_, err := f.a.Exec(f.ctx, `UPDATE problem_drafts SET created_at='2026-11-03T00:00:00.123456Z'::timestamptz`)
	must(t, err)
	seen := make(map[string]bool)
	var after *problem.ListPosition
	for {
		rows, e := f.repository.ListOwn(f.ctx, f.actor, after)
		must(t, e)
		for _, r := range rows[:min(len(rows), problem.OwnDraftPageSize)] {
			if seen[r.DraftID] || after != nil && r.DraftID >= after.ID {
				t.Fatal("equal-time seek duplicated/reordered a row")
			}
			seen[r.DraftID] = true
		}
		if len(rows) <= problem.OwnDraftPageSize {
			break
		}
		last := rows[problem.OwnDraftPageSize-1]
		after = &problem.ListPosition{CreatedAt: last.CreatedAt, ID: last.DraftID}
	}
	if len(seen) != 45 {
		t.Fatal("equal-time pagination lost committed rows")
	}
}

func TestOwnDraftListRejectsInvalidStoredTimestamps(t *testing.T) {
	for _, timestamp := range []string{"infinity", "10000-01-01 00:00:00+00"} {
		t.Run(timestamp, func(t *testing.T) {
			f := newDraftFixture(t)
			f.role(t, f.actor.SubjectID, 0, true)
			insertListRows(t, f, f.actor.SubjectID, 2)
			_, err := f.a.Exec(f.ctx, `UPDATE problem_drafts SET created_at=$1::timestamptz WHERE id=(SELECT id FROM problem_drafts LIMIT 1)`, timestamp)
			must(t, err)
			rows, err := f.repository.ListOwn(f.ctx, f.actor, nil)
			if !errors.Is(err, problem.ErrStore) || rows != nil {
				t.Fatal("invalid stored timestamp returned a partial list", err)
			}
		})
	}
}

func TestOwnDraftListAcrossConcurrentCommitsIsNotSnapshot(t *testing.T) {
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	insertListRows(t, f, f.actor.SubjectID, 45)
	lateID := strings.Repeat("f", 32)
	newID := strings.Repeat("e", 32)
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer tx.Rollback(f.ctx)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_drafts(id,owner_subject_id,request_key,request_sha256,source_id,content_version,document,created_at)
		VALUES ($1,$2,$3,repeat('a',64),'synthetic',repeat('b',64),'x'::bytea,$4)`, lateID, f.actor.SubjectID, "late-commit-list-key", time.Date(2026, 1, 1, 0, 0, 0, 12000, time.UTC))
	must(t, err)
	first, err := f.repository.ListOwn(f.ctx, f.actor, nil)
	must(t, err)
	if len(first) != 21 {
		t.Fatal("fixture did not fill first page")
	}
	boundary := &problem.ListPosition{CreatedAt: first[19].CreatedAt, ID: first[19].DraftID}
	second, err := f.repository.ListOwn(f.ctx, f.actor, boundary)
	must(t, err)
	if len(second) != 21 {
		t.Fatal("fixture did not fill second page")
	}
	for _, page := range [][]problem.ImportReceipt{first, second} {
		for _, row := range page {
			if row.DraftID == lateID {
				t.Fatal("uncommitted insert was visible")
			}
		}
	}
	must(t, tx.Commit(f.ctx))
	_, err = f.a.Exec(f.ctx, `INSERT INTO problem_drafts(id,owner_subject_id,request_key,request_sha256,source_id,content_version,document,created_at)
		VALUES ($1,$2,$3,repeat('a',64),'synthetic',repeat('b',64),'x'::bytea,$4)`, newID, f.actor.SubjectID, "new-commit-list-key", first[0].CreatedAt.Add(time.Second))
	must(t, err)
	continued, err := f.repository.ListOwn(f.ctx, f.actor, boundary)
	must(t, err)
	prior := make(map[string]bool)
	for _, row := range first[:20] {
		prior[row.DraftID] = true
	}
	for _, row := range continued {
		if row.DraftID == lateID || row.DraftID == newID || prior[row.DraftID] {
			t.Fatal("new or previously read row crossed the old boundary")
		}
	}
	refreshed, err := f.repository.ListOwn(f.ctx, f.actor, nil)
	must(t, err)
	seenLate, seenNew := false, false
	for _, row := range refreshed {
		seenLate = seenLate || row.DraftID == lateID
		seenNew = seenNew || row.DraftID == newID
	}
	if !seenLate || !seenNew || refreshed[0].DraftID != newID {
		t.Fatal("fresh first page failed to reveal committed inserts")
	}
}

func TestOwnDraftListQueryPlans(t *testing.T) {
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	insertListRows(t, f, f.actor.SubjectID, 6000)
	insertListRows(t, f, identityID(1101), 6000)
	_, err := f.a.Exec(f.ctx, `ANALYZE problem_drafts`)
	must(t, err)
	raw, err := os.ReadFile("../../db/queries/problem/list.sql")
	must(t, err)
	for _, test := range []struct {
		name string
		args []any
	}{
		{"ListOwnDraftsFirst", []any{f.actor.SubjectID, int32(21)}},
		{"ListOwnDraftsAfter", []any{f.actor.SubjectID, time.Date(2026, 1, 1, 0, 0, 0, 1000, time.UTC), strings.Repeat("f", 32), int32(21)}},
	} {
		parts := strings.Split(string(raw), "-- name: "+test.name+" :many\n")
		if len(parts) != 2 {
			t.Fatal("missing unique query")
		}
		query := strings.Split(parts[1], "\n-- name:")[0]
		if test.name == "ListOwnDraftsFirst" {
			query = strings.NewReplacer("sqlc.arg(owner)", "$1", "sqlc.arg(row_limit)", "$2").Replace(query)
		} else {
			query = strings.NewReplacer("sqlc.arg(owner)", "$1", "sqlc.arg(created_at)", "$2", "sqlc.arg(id)", "$3", "sqlc.arg(row_limit)", "$4").Replace(query)
		}
		var plan string
		must(t, f.a.QueryRow(f.ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+query, test.args...).Scan(&plan))
		if !strings.Contains(plan, "problem_drafts_owner_created_id_idx") || !strings.Contains(plan, "Index Cond") || strings.Contains(plan, `"Node Type": "Sort"`) {
			t.Fatalf("unexpected plan %s: %s", test.name, plan)
		}
		if test.name == "ListOwnDraftsAfter" && !strings.Contains(plan, "ROW(created_at") {
			t.Fatalf("seek missing from index condition: %s", plan)
		}
		t.Log(test.name, plan)
	}
	var bytes int64
	must(t, f.a.QueryRow(f.ctx, `SELECT pg_relation_size('problem_drafts_owner_created_id_idx')`).Scan(&bytes))
	t.Logf("index size on 12000 synthetic rows: %d bytes", bytes)
}

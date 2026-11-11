//go:build integration

package integration_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/jackc/pgx/v5"
)

type reviewFixture struct {
	expectedPlanFixture
	reviews *referencepg.ReviewStore
	item    problemreference.ReviewReceipt
	reader  identity.ProblemActor
}

func newReviewFixture(t *testing.T) reviewFixture {
	t.Helper()
	f, claims, node, report := acceptedReportFixture(t, "WA")
	must(t, claims.ReportReference(f.ctx, node, report))
	item, err := f.store.SubmitReview(f.ctx, f.actor, reviewIntent(f))
	must(t, err)
	reviews, err := referencepg.NewReviewStore(f.b, f.access)
	must(t, err)
	reader := f.actor
	reader.SubjectID = identityID(1101)
	return reviewFixture{expectedPlanFixture: f, reviews: reviews, item: item, reader: reader}
}

func (f reviewFixture) grant(t *testing.T, actor identity.ProblemActor, expected int64, enabled bool) {
	t.Helper()
	role, err := f.access.SetReviewerRole(f.ctx, identity.ProblemRoleChange{SubjectID: actor.SubjectID, ExpectedRevision: expected,
		Enabled: enabled, Operator: "review-fixture", Reason: "private test role change"})
	must(t, err)
	if role.SubjectID != actor.SubjectID || role.Enabled != enabled || role.Revision != expected+1 {
		t.Fatal("reviewer role CAS returned wrong revision")
	}
}

func (f reviewFixture) change(expected int64, target string, roleRevision int64) problemreference.ReviewAssignmentChange {
	return problemreference.ReviewAssignmentChange{ReviewID: f.item.ID, ExpectedRevision: expected,
		ReviewerSubjectID: target, ExpectedReviewerRevision: roleRevision, Operator: "review-fixture", Reason: "private test assignment"}
}

func (f reviewFixture) reject(t *testing.T, actor identity.ProblemActor, id string, want error) {
	t.Helper()
	got, err := f.reviews.ReviewEvidence(f.ctx, actor, id)
	requireReviewError(t, err, want)
	if got != (problemreference.ReviewEvidence{}) {
		t.Fatal("denied read returned private evidence")
	}
}

func TestReviewAccessAssignmentAndRevocation(t *testing.T) {
	f := newReviewFixture(t)
	f.role(t, f.reader.SubjectID, 0, true)
	f.reject(t, f.reader, f.item.ID, identity.ErrReviewerRequired)
	f.grant(t, f.reader, 0, true)
	f.grant(t, f.actor, 0, true)
	f.reject(t, f.reader, f.item.ID, problemreference.ErrNotFound)
	f.reject(t, f.actor, f.item.ID, problemreference.ErrNotFound)
	_, err := f.reviews.SetAssignment(f.ctx, f.change(0, f.actor.SubjectID, 1))
	requireReviewError(t, err, identity.ErrReviewerRequired)
	_, err = f.reviews.SetAssignment(f.ctx, f.change(0, f.reader.SubjectID, 2))
	requireReviewError(t, err, problemreference.ErrConflict)

	a, err := f.reviews.SetAssignment(f.ctx, f.change(0, f.reader.SubjectID, 1))
	must(t, err)
	e, err := f.reviews.ReviewEvidence(f.ctx, f.reader, f.item.ID)
	must(t, err)
	if !e.Valid() || e.ReviewID != f.item.ID || e.DraftID != f.draft.ID || e.ImportRevision != f.draft.ImportRevision ||
		e.FormalPlanSHA256 != f.want.SHA256 || e.FormalVerdict != "WA" || e.AssignmentRevision != a.Revision {
		t.Fatal("reviewer saw another revision or evidence")
	}
	f.reject(t, f.reader, f.request.ID, problemreference.ErrNotFound)
	f.grant(t, f.reader, 1, false)
	f.reject(t, f.reader, f.item.ID, identity.ErrReviewerRequired)
	f.grant(t, f.reader, 2, true)
	f.reject(t, f.reader, f.item.ID, problemreference.ErrNotFound)
	_, err = f.reviews.SetAssignment(f.ctx, f.change(1, f.reader.SubjectID, 1))
	requireReviewError(t, err, problemreference.ErrConflict)

	res := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := f.reviews.SetAssignment(f.ctx, f.change(1, f.reader.SubjectID, 3))
			res <- err
		}()
	}
	aErr, bErr := <-res, <-res
	if !((aErr == nil && errors.Is(bErr, problemreference.ErrConflict)) || (bErr == nil && errors.Is(aErr, problemreference.ErrConflict))) {
		t.Fatalf("same revision produced unexpected outcomes: %v, %v", aErr, bErr)
	}
	e, err = f.reviews.ReviewEvidence(f.ctx, f.reader, f.item.ID)
	must(t, err)
	if e.AssignmentRevision != 2 {
		t.Fatal("the old assignment was reused")
	}
	_, err = f.reviews.SetAssignment(f.ctx, f.change(2, "", 0))
	must(t, err)
	f.reject(t, f.reader, f.item.ID, problemreference.ErrNotFound)
	a, err = f.reviews.Assignment(f.ctx, f.item.ID)
	must(t, err)
	if a.Revision != 3 || a.ReviewerSubjectID != "" {
		t.Fatal("withdrawal did not preserve a visible revision")
	}
	_, err = f.provider.DownTo(f.ctx, 16)
	must(t, err)
	if _, err := f.provider.DownTo(f.ctx, 15); err == nil || !strings.Contains(err.Error(), "cannot discard review access history") {
		t.Fatalf("nonempty review access history could be dropped: %v", err)
	}
	// DownTo is not atomic across versions. Reapply the empty newer ledger
	// before checking the current binary's exact startup version.
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
}

func TestReviewAccessSeesWithdrawalAfterLockWait(t *testing.T) {
	for _, boundary := range []string{"role", "assignment"} {
		t.Run(boundary, func(t *testing.T) {
			f := newReviewFixture(t)
			f.grant(t, f.reader, 0, true)
			_, err := f.reviews.SetAssignment(f.ctx, f.change(0, f.reader.SubjectID, 1))
			must(t, err)
			tx, err := f.a.BeginTx(f.ctx, pgx.TxOptions{})
			must(t, err)
			defer func() { _ = tx.Rollback(f.ctx) }()
			var holder uint32
			must(t, tx.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&holder))
			var id string
			if boundary == "role" {
				must(t, tx.QueryRow(f.ctx, `SELECT id FROM identity_credentials WHERE id=$1 FOR UPDATE`, f.reader.SubjectID).Scan(&id))
			} else {
				must(t, tx.QueryRow(f.ctx, `SELECT id FROM problem_review_submissions WHERE id=$1 FOR UPDATE`, f.item.ID).Scan(&id))
			}
			type result struct {
				view problemreference.ReviewEvidence
				err  error
			}
			got := make(chan result, 1)
			go func() {
				e, err := f.reviews.ReviewEvidence(f.ctx, f.reader, f.item.ID)
				got <- result{e, err}
			}()
			waitBacklogBlocked(t, f.backlogFixture, holder, f.appB)
			want := error(problemreference.ErrNotFound)
			if boundary == "role" {
				_, err = tx.Exec(f.ctx, `INSERT INTO identity_problem_reviewer_history
					(subject_id,revision,enabled,maintenance_operator,reason) VALUES($1,2,false,'fixture','revoke')`, id)
				want = identity.ErrReviewerRequired
			} else {
				_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_assignment_history
					(review_id,revision,maintenance_operator,reason) VALUES($1,2,'fixture','withdraw')`, id)
			}
			must(t, err)
			must(t, tx.Commit(f.ctx))
			r := <-got
			requireReviewError(t, r.err, want)
			if r.view != (problemreference.ReviewEvidence{}) {
				t.Fatal("waiter reused stale authority")
			}
		})
	}
}

func TestReviewAccessEmptyRollbackPreservesIntake(t *testing.T) {
	f := newReviewFixture(t)
	_, err := f.provider.DownTo(f.ctx, 15)
	must(t, err)
	if db.CheckSchema(f.ctx, f.a) == nil {
		t.Fatal("current startup accepted schema without review authorization")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
	receipt, err := f.store.ReviewReceipt(f.ctx, f.actor, f.item.RequestKey)
	must(t, err)
	if receipt != f.item {
		t.Fatal("empty review authorization rollback changed an intake")
	}
}

func TestReviewAccessRollbackWaitsForWithdrawal(t *testing.T) {
	f := newReviewFixture(t)
	f.grant(t, f.reader, 0, true)
	_, err := f.reviews.SetAssignment(f.ctx, f.change(0, f.reader.SubjectID, 1))
	must(t, err)
	_, err = f.provider.DownTo(f.ctx, 16)
	must(t, err)
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	var holder uint32
	var id string
	must(t, tx.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&holder))
	must(t, tx.QueryRow(f.ctx, `SELECT id FROM problem_review_submissions WHERE id=$1 FOR UPDATE`, f.item.ID).Scan(&id))
	down := make(chan error, 1)
	go func() { _, err := f.provider.DownTo(f.ctx, 15); down <- err }()
	watch, cancel := context.WithTimeout(f.ctx, 2*time.Second)
	defer cancel()
	for {
		var waiting bool
		must(t, f.b.QueryRow(watch, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)))`, holder).Scan(&waiting))
		if waiting {
			break
		}
		select {
		case <-watch.Done():
			t.Fatal("migration never waited for the withdrawal")
		case <-time.After(5 * time.Millisecond):
		}
	}
	// Even the NULL reviewer FK opens its parent table on INSERT. The
	// waiting migration must not already hold that table exclusively.
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_assignment_history
		(review_id,revision,maintenance_operator,reason) VALUES($1,2,'fixture','withdraw')`, id)
	must(t, err)
	must(t, tx.Commit(f.ctx))
	if err := <-down; err == nil || !strings.Contains(err.Error(), "cannot discard review access history") {
		t.Fatalf("migration must see committed history and refuse: %v", err)
	}
	a, err := f.reviews.Assignment(f.ctx, f.item.ID)
	must(t, err)
	if a.Revision != 2 || a.ReviewerSubjectID != "" {
		t.Fatal("rollback changed the withdrawal")
	}
}

func TestReviewAccessLatestLookupsUsePrimaryKeys(t *testing.T) {
	f := newReviewFixture(t)
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `INSERT INTO identity_problem_reviewer_history
		(subject_id,revision,enabled,maintenance_operator,reason)
		SELECT $1,n,true,'fixture','synthetic history' FROM generate_series(1,2048) n`, f.reader.SubjectID)
	must(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_assignment_history
		(review_id,revision,reviewer_subject_id,reviewer_revision,maintenance_operator,reason)
		SELECT $1,n,$2,n,'fixture','synthetic history' FROM generate_series(1,2048) n`, f.item.ID, f.reader.SubjectID)
	must(t, err)
	_, err = tx.Exec(f.ctx, `ANALYZE identity_problem_reviewer_history; ANALYZE problem_review_assignment_history`)
	must(t, err)
	for _, tc := range []struct{ file, query, arg, key, id string }{
		{"identity/problem_reviewers.sql", "LatestProblemReviewer", "@subject_id", "identity_problem_reviewer_history_pkey", f.reader.SubjectID},
		{"problemreference/review_access.sql", "LatestReviewAssignment", "@review_id", "problem_review_assignment_history_pkey", f.item.ID},
	} {
		raw, err := os.ReadFile("../../db/queries/" + tc.file)
		must(t, err)
		_, tail, ok := strings.Cut(string(raw), "-- name: "+tc.query+" :one\n")
		if !ok {
			t.Fatal("missing authoritative latest revision query")
		}
		sql, _, _ := strings.Cut(tail, "\n-- name:")
		var plan string
		must(t, tx.QueryRow(f.ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+strings.ReplaceAll(sql, tc.arg, "$1"), tc.id).Scan(&plan))
		if !strings.Contains(plan, tc.key) || !strings.Contains(plan, `"Scan Direction": "Backward"`) ||
			!strings.Contains(plan, `"Actual Rows": 1`) || strings.Contains(plan, `"Node Type": "Sort"`) {
			t.Fatal("latest revision did not use the ordered primary key", plan)
		}
		t.Log(tc.query, plan)
	}
}

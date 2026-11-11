//go:build integration

package integration_test

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
)

func statementIntent(e problemreference.ReviewEvidence, key string) problemreference.StatementConfirmationIntent {
	return problemreference.StatementConfirmationIntent{
		RequestKey: key, AssignmentRevision: e.AssignmentRevision, ImportRevision: e.ImportRevision,
		EvidenceSHA256: e.EvidenceSHA256, ChecklistVersion: problemreference.StatementChecklistVersion,
	}
}

func TestStatementConfirmationAssignmentAndSharedReturn(t *testing.T) {
	f := newReviewFixture(t)
	if _, err := f.provider.DownTo(f.ctx, 18); err != nil {
		t.Fatalf("empty ledger did not roll back: %v", err)
	}
	if err := db.CheckSchema(f.ctx, f.a); err == nil {
		t.Fatal("schema 18 unexpectedly accepted")
	}
	_, err := f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
	f.grant(t, f.actor, 0, true)
	f.grant(t, f.reader, 0, true)
	e := assignReview(t, f, f.item.ID)
	i := statementIntent(e, "statement-original-assignment")
	s, err := referencepg.NewReviewMaterialStore(f.reviews, problempg.ReadBoundReviewDraft)
	must(t, err)
	before, err := s.ReviewMaterialSnapshot(f.ctx, f.reader, e.ReviewID)
	must(t, err)
	_, err = s.RecordStatementConfirmation(f.ctx, f.actor, e.ReviewID, i, before)
	requireReviewError(t, err, problemreference.ErrNotFound)
	f.grant(t, f.reader, 1, false)
	_, err = s.RecordStatementConfirmation(f.ctx, f.reader, e.ReviewID, i, before)
	requireReviewError(t, err, identity.ErrReviewerRequired)
	f.grant(t, f.reader, 2, true)
	_, err = s.RecordStatementConfirmation(f.ctx, f.reader, e.ReviewID, i, before)
	requireReviewError(t, err, problemreference.ErrNotFound)
	_, err = f.reviews.SetAssignment(f.ctx, f.change(1, f.reader.SubjectID, 3))
	must(t, err)
	_, err = s.RecordStatementConfirmation(f.ctx, f.reader, e.ReviewID, i, before)
	requireReviewError(t, err, problemreference.ErrReviewChanged)
	before, err = s.ReviewMaterialSnapshot(f.ctx, f.reader, e.ReviewID)
	must(t, err)
	e = before.Evidence
	i = statementIntent(e, "statement-live-assignment")

	duplicate := f.item.ReviewIntent
	duplicate.RequestKey = "statement-duplicate-intake"
	intake, err := f.store.SubmitReview(f.ctx, f.actor, duplicate)
	must(t, err)
	change := f.change(0, f.reader.SubjectID, 3)
	change.ReviewID = intake.ID
	_, err = f.reviews.SetAssignment(f.ctx, change)
	must(t, err)
	other, err := s.ReviewMaterialSnapshot(f.ctx, f.reader, intake.ID)
	must(t, err)

	type result struct {
		receipt problemreference.StatementConfirmationReceipt
		err     error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			r, err := s.RecordStatementConfirmation(f.ctx, f.reader, e.ReviewID, i, before)
			results <- result{r, err}
		}()
	}
	a, b := <-results, <-results
	must(t, a.err)
	must(t, b.err)
	if !a.receipt.Valid() || a.receipt != b.receipt {
		t.Fatal("same intent did not replay the original receipt")
	}
	_, err = s.RecordStatementConfirmation(f.ctx, f.reader, e.ReviewID, statementIntent(e, "statement-different-key"), before)
	requireReviewError(t, err, problemreference.ErrConflict)
	_, err = s.RecordStatementConfirmation(f.ctx, f.reader, intake.ID, statementIntent(other.Evidence, i.RequestKey), other)
	requireReviewError(t, err, problemreference.ErrConflict)
	snapshot, err := f.reviews.ReviewEligibilitySnapshot(f.ctx, f.reader, e.ReviewID)
	must(t, err)
	if !snapshot.StatementConfirmed || snapshot.Returned {
		t.Fatal("current assignment did not observe its confirmation")
	}
	// A return on a different intake for the same material wins over any
	// existing semantic confirmation and forbids a new one.
	_, err = f.reviews.ReturnReview(f.ctx, f.reader, intake.ID, returnIntent(other.Evidence, "statement-duplicate-return"))
	must(t, err)
	_, err = s.RecordStatementConfirmation(f.ctx, f.reader, intake.ID, statementIntent(other.Evidence, "statement-after-return"), other)
	requireReviewError(t, err, problemreference.ErrReviewReturned)
	snapshot, err = f.reviews.ReviewEligibilitySnapshot(f.ctx, f.reader, e.ReviewID)
	must(t, err)
	if snapshot.StatementConfirmed || !snapshot.Returned {
		t.Fatal("duplicate return left the old confirmation effective")
	}
	recovered, err := s.StatementConfirmationReceipt(f.ctx, f.reader, e.ReviewID, i.RequestKey)
	must(t, err)
	if recovered != a.receipt {
		t.Fatal("return rewrote the historical receipt")
	}
	_, err = f.reviews.SetAssignment(f.ctx, f.change(e.AssignmentRevision, f.reader.SubjectID, 3))
	must(t, err)
	_, err = s.StatementConfirmationReceipt(f.ctx, f.reader, e.ReviewID, i.RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)
	if _, err := f.provider.DownTo(f.ctx, 18); err == nil || !strings.Contains(err.Error(), "cannot discard statement confirmations") {
		t.Fatalf("nonempty rollback did not preserve history: %v", err)
	}
	var count int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM problem_statement_confirmations`).Scan(&count))
	if count != 1 {
		t.Fatalf("confirmation ledger contains %d rows, want 1", count)
	}
}

func TestStatementConfirmationRecoversLostCommitAndUsesIndexes(t *testing.T) {
	f := newReviewFixture(t)
	f.grant(t, f.reader, 0, true)
	e := assignReview(t, f, f.item.ID)
	i := statementIntent(e, "statement-lost-commit")
	s, err := referencepg.NewReviewMaterialStore(f.reviews, problempg.ReadBoundReviewDraft)
	must(t, err)
	before, err := s.ReviewMaterialSnapshot(f.ctx, f.reader, e.ReviewID)
	must(t, err)
	pool, observed := lostReferenceCommitPool(t, f.referenceRequestFixture)
	reviews, err := referencepg.NewReviewStore(pool, f.access)
	must(t, err)
	fault, err := referencepg.NewReviewMaterialStore(reviews, problempg.ReadBoundReviewDraft)
	must(t, err)
	r, err := fault.RecordStatementConfirmation(f.ctx, f.reader, e.ReviewID, i, before)
	requireReviewError(t, err, problemreference.ErrCommitUnknown)
	if r != (problemreference.StatementConfirmationReceipt{}) {
		t.Fatal("unknown COMMIT returned an unconfirmed receipt")
	}
	select {
	case committed := <-observed:
		if !committed {
			t.Fatal("did not lose a real COMMIT reply")
		}
	case <-f.ctx.Done():
		t.Fatal("COMMIT fault did not run")
	}
	r, err = s.StatementConfirmationReceipt(f.ctx, f.reader, e.ReviewID, i.RequestKey)
	must(t, err)
	replay, err := s.RecordStatementConfirmation(f.ctx, f.reader, e.ReviewID, i, before)
	must(t, err)
	if r != replay || r.StatementConfirmationIntent != i {
		t.Fatal("recovery changed the original receipt")
	}
	assertStatementConfirmationIndexes(t, f, r)
	_, err = f.reviews.SetAssignment(f.ctx, f.change(e.AssignmentRevision, f.reader.SubjectID, 1))
	must(t, err)
	current, err := f.reviews.ReviewEligibilitySnapshot(f.ctx, f.reader, e.ReviewID)
	must(t, err)
	if current.Returned || current.StatementConfirmed {
		t.Fatal("same-person reassignment inherited a previous confirmation")
	}
	_, err = s.StatementConfirmationReceipt(f.ctx, f.reader, e.ReviewID, i.RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)
}

func TestStatementConfirmationSeesDuplicateReturnAfterMaterialLock(t *testing.T) {
	f := newReviewFixture(t)
	f.grant(t, f.reader, 0, true)
	e := assignReview(t, f, f.item.ID)
	intake := f.item.ReviewIntent
	intake.RequestKey = "statement-concurrent-return-intake"
	duplicate, err := f.store.SubmitReview(f.ctx, f.actor, intake)
	must(t, err)
	other := assignReview(t, f, duplicate.ID)
	s, err := referencepg.NewReviewMaterialStore(f.reviews, problempg.ReadBoundReviewDraft)
	must(t, err)
	before, err := s.ReviewMaterialSnapshot(f.ctx, f.reader, e.ReviewID)
	must(t, err)
	i := statementIntent(e, "statement-wait-for-return")
	// Install a return on a different intake and hold its transaction open.
	// The real confirmation must block on this same material lock.
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	var holder uint32
	must(t, tx.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&holder))
	digest := sha256.Sum256([]byte("onlinejudge/review-material/v1:" + f.actor.SubjectID +
		e.ImportRevision + e.FormalPlanSHA256 + e.ProfileSHA256 + e.EvidenceSHA256 + e.ComparisonPlan))
	_, err = tx.Exec(f.ctx, `SELECT pg_advisory_xact_lock($1::bigint)`, int64(binary.BigEndian.Uint64(digest[:8])))
	must(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_returns
		(review_id,material_sha256,reviewer_subject_id,request_key,reason,auth_generation,recovery_id,reviewer_revision,assignment_revision)
		VALUES($1,$2,$3,'statement-return-on-duplicate','fixture',$4,$5,1,$6)`,
		other.ReviewID, hex.EncodeToString(digest[:]), f.reader.SubjectID, f.reader.AuthGeneration, f.reader.RecoveryID, other.AssignmentRevision)
	must(t, err)
	done := make(chan error, 1)
	go func() {
		_, err := s.RecordStatementConfirmation(f.ctx, f.reader, e.ReviewID, i, before)
		done <- err
	}()
	waitBacklogBlocked(t, f.backlogFixture, holder, f.appB)
	_, err = s.StatementConfirmationReceipt(f.ctx, f.reader, e.ReviewID, i.RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)
	must(t, tx.Commit(f.ctx))
	requireReviewError(t, <-done, problemreference.ErrReviewReturned)
	_, err = s.StatementConfirmationReceipt(f.ctx, f.reader, e.ReviewID, i.RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)
}

func assertStatementConfirmationIndexes(t *testing.T, f reviewFixture, r problemreference.StatementConfirmationReceipt) {
	t.Helper()
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_assignment_history
		(review_id,revision,reviewer_subject_id,reviewer_revision,maintenance_operator,reason)
		SELECT review_id,n+1,reviewer_subject_id,reviewer_revision,'fixture','plan'
		FROM problem_statement_confirmations CROSS JOIN generate_series(1,2048) n WHERE review_id=$1`, r.ReviewID)
	must(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_statement_confirmations
		(review_id,assignment_revision,reviewer_subject_id,reviewer_revision,request_key,
		import_revision,evidence_sha256,checklist_version,auth_generation,recovery_id)
		SELECT review_id,n+1,reviewer_subject_id,reviewer_revision,'statement-plan-fixture-'||n,
		import_revision,evidence_sha256,checklist_version,auth_generation,recovery_id
		FROM problem_statement_confirmations CROSS JOIN generate_series(1,2048) n WHERE review_id=$1`, r.ReviewID)
	must(t, err)
	_, err = tx.Exec(f.ctx, `ANALYZE problem_statement_confirmations`)
	must(t, err)
	raw, err := os.ReadFile("../../db/queries/problemreference/statement_confirmations.sql")
	must(t, err)
	for _, tc := range []struct {
		name, index string
		params      *strings.Replacer
		args        []any
	}{
		{"StatementConfirmationByAssignment", "problem_statement_confirmations_pkey",
			strings.NewReplacer("@review_id", "$1", "@assignment_revision", "$2"), []any{r.ReviewID, r.AssignmentRevision}},
		{"StatementConfirmationByKey", "problem_statement_confirmations_reviewer_key",
			strings.NewReplacer("@reviewer_subject_id", "$1", "@request_key", "$2"), []any{r.Reviewer, r.RequestKey}},
	} {
		_, rest, found := strings.Cut(string(raw), "-- name: "+tc.name+" :one\n")
		if !found {
			t.Fatal("query not found:", tc.name)
		}
		query, _, _ := strings.Cut(rest, ";")
		rows, err := tx.Query(f.ctx, "EXPLAIN (ANALYZE, COSTS OFF) "+tc.params.Replace(query), tc.args...)
		must(t, err)
		var plan strings.Builder
		for rows.Next() {
			var line string
			must(t, rows.Scan(&line))
			plan.WriteString(line + "\n")
		}
		rows.Close()
		must(t, rows.Err())
		if !strings.Contains(plan.String(), "Index Scan using "+tc.index) || !strings.Contains(plan.String(), "rows=1 loops=1") {
			t.Fatalf("%s did not use its unique index:\n%s", tc.name, plan.String())
		}
		t.Logf("2,049 confirmations; %s:\n%s", tc.name, plan.String())
	}
}

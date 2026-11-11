//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
)

func TestAuthorSolutionConcurrentConfirmation(t *testing.T) {
	f := newReviewFixture(t)
	solutions, err := referencepg.NewAuthorSolutionStore(f.store, problempg.ReadBoundReviewDraft)
	must(t, err)
	duplicate := f.item.ReviewIntent
	duplicate.RequestKey = "solution-confirmation-duplicate"
	same, err := f.store.SubmitReview(f.ctx, f.actor, duplicate)
	must(t, err)
	f.role(t, f.reader.SubjectID, 0, true)
	f.grant(t, f.reader, 0, true)
	e := assignReview(t, f, f.item.ID)
	_, err = solutions.AuthorSolutionSnapshot(f.ctx, f.reader, f.item.RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)
	first, err := solutions.AuthorSolutionSnapshot(f.ctx, f.actor, f.item.RequestKey)
	must(t, err)
	f.role(t, f.actor.SubjectID, 1, false)
	_, err = solutions.ConfirmAuthorSolution(f.ctx, f.actor, f.item.RequestKey, first)
	requireReviewError(t, err, identity.ErrForbidden)
	f.role(t, f.actor.SubjectID, 2, true)
	_, err = solutions.ConfirmAuthorSolution(f.ctx, f.actor, f.item.RequestKey, first)
	requireReviewError(t, err, problemreference.ErrReviewChanged)
	first, err = solutions.AuthorSolutionSnapshot(f.ctx, f.actor, f.item.RequestKey)
	must(t, err)
	second, err := solutions.AuthorSolutionSnapshot(f.ctx, f.actor, same.RequestKey)
	must(t, err)
	before, err := f.reviews.ReviewEligibilitySnapshot(f.ctx, f.reader, f.item.ID)
	must(t, err)
	if before.SolutionConfirmed || first.Binding != second.Binding || first.Binding.FormalPlanSHA256 != e.FormalPlanSHA256 {
		t.Fatal("intake or reference execution counted as a solution confirmation")
	}
	bad := first
	bad.Binding.ImportRevision = strings.Repeat("b", 64)
	_, err = solutions.ConfirmAuthorSolution(f.ctx, f.actor, f.item.RequestKey, bad)
	requireReviewError(t, err, problemreference.ErrReviewChanged)
	type result struct {
		receipt problemreference.SolutionConfirmation
		err     error
	}
	results := make(chan result, 2)
	for i, selection := range []problemreference.AuthorSolutionSnapshot{first, second} {
		key := []string{f.item.RequestKey, same.RequestKey}[i]
		go func() {
			r, err := solutions.ConfirmAuthorSolution(f.ctx, f.actor, key, selection)
			results <- result{r, err}
		}()
	}
	a, b := <-results, <-results
	must(t, a.err)
	must(t, b.err)
	if !a.receipt.Valid() || !b.receipt.Valid() || a.receipt.IntakeKey == b.receipt.IntakeKey ||
		a.receipt.SolutionBinding != b.receipt.SolutionBinding || a.receipt.ConfirmedAt != b.receipt.ConfirmedAt {
		t.Fatal("same solution received two confirmations")
	}
	after, err := f.reviews.ReviewEligibilitySnapshot(f.ctx, f.reader, f.item.ID)
	must(t, err)
	if !after.SolutionConfirmed || after.Evidence != before.Evidence || after.Returned {
		t.Fatal("confirmation did not resolve only the same solution prerequisite")
	}
	var count int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM problem_solution_confirmations`).Scan(&count))
	if count != 1 {
		t.Fatalf("confirmations=%d, want one", count)
	}
	if _, err := f.provider.DownTo(f.ctx, 17); err == nil || !strings.Contains(err.Error(), "cannot discard solution confirmations") {
		t.Fatalf("Down18 did not preserve the confirmation: %v", err)
	}
	_, err = f.a.Exec(f.ctx, `UPDATE problem_drafts SET request_sha256=$1 WHERE id=$2`, strings.Repeat("c", 64), first.DraftID)
	must(t, err)
	_, err = solutions.ConfirmAuthorSolution(f.ctx, f.actor, f.item.RequestKey, first)
	requireReviewError(t, err, problemreference.ErrReviewChanged)
}

func TestAuthorSolutionRecoversLostCommitAndIndexedLookup(t *testing.T) {
	f := newReviewFixture(t)
	solutions, err := referencepg.NewAuthorSolutionStore(f.store, problempg.ReadBoundReviewDraft)
	must(t, err)
	selection, err := solutions.AuthorSolutionSnapshot(f.ctx, f.actor, f.item.RequestKey)
	must(t, err)
	pool, observed := lostReferenceCommitPool(t, f.referenceRequestFixture)
	access, err := problempg.NewReferenceAccess(f.access)
	must(t, err)
	faultBase, err := referencepg.New(pool, access)
	must(t, err)
	fault, err := referencepg.NewAuthorSolutionStore(faultBase, problempg.ReadBoundReviewDraft)
	must(t, err)
	r, err := fault.ConfirmAuthorSolution(f.ctx, f.actor, f.item.RequestKey, selection)
	requireReviewError(t, err, problemreference.ErrCommitUnknown)
	if r != (problemreference.SolutionConfirmation{}) {
		t.Fatal("unknown write returned a receipt")
	}
	select {
	case committed := <-observed:
		if !committed {
			t.Fatal("did not lose a real COMMIT reply")
		}
	case <-f.ctx.Done():
		t.Fatal("COMMIT fault did not run")
	}
	recovered, err := solutions.AuthorSolutionConfirmation(f.ctx, f.actor, f.item.RequestKey)
	must(t, err)
	if !recovered.Valid() || recovered.SolutionBinding != selection.Binding {
		t.Fatal("receipt recovery lost the original binding")
	}
	assertSolutionIndex(t, f, selection)
}

func assertSolutionIndex(t *testing.T, f reviewFixture, s problemreference.AuthorSolutionSnapshot) {
	t.Helper()
	_, err := f.a.Exec(f.ctx, `INSERT INTO problem_solution_confirmations
		(draft_id, import_revision, formal_plan_sha256, review_id, owner_subject_id, author_revision, auth_generation, recovery_id)
		SELECT lpad(to_hex(i), 32, '0'), import_revision, formal_plan_sha256, review_id, owner_subject_id,
			author_revision, auth_generation, recovery_id
		FROM problem_solution_confirmations CROSS JOIN generate_series(1,2048) i`)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `ANALYZE problem_solution_confirmations`)
	must(t, err)
	rows, err := f.a.Query(f.ctx, `EXPLAIN (ANALYZE, COSTS OFF)
		SELECT * FROM problem_solution_confirmations WHERE draft_id=$1 AND import_revision=$2 AND formal_plan_sha256=$3`,
		s.DraftID, s.Binding.ImportRevision, s.Binding.FormalPlanSHA256)
	must(t, err)
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		must(t, rows.Scan(&line))
		plan.WriteString(line + "\n")
	}
	must(t, rows.Err())
	if !strings.Contains(plan.String(), "Index Scan using problem_solution_confirmations_pkey") || !strings.Contains(plan.String(), "rows=1 loops=1") {
		t.Fatalf("confirmation lookup did not use its exact binding index:\n%s", plan.String())
	}
	t.Logf("2,049 confirmation rows; primary-key lookup:\n%s", plan.String())
}

//go:build integration

package integration_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

func TestReviewIntakeSchemaRequires15(t *testing.T) {
	f := newBacklogFixture(t)
	must(t, db.CheckSchema(f.ctx, f.a))
	_, err := f.provider.DownTo(f.ctx, 14)
	must(t, err)
	if db.CheckSchema(f.ctx, f.a) == nil {
		t.Fatal("startup accepted schema without the intake ledger")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
}

func TestReviewIntakeBindsAcceptedReportAndOwner(t *testing.T) {
	f, claims, node, report := acceptedReportFixture(t, "WA")
	intent := reviewIntent(f)
	_, err := f.store.SubmitReview(f.ctx, f.actor, intent)
	requireReviewError(t, err, problemreference.ErrNotFound)
	must(t, claims.ReportReference(f.ctx, node, report))
	stale := intent
	stale.ImportRevision = strings.Repeat("f", 64)
	_, err = f.store.SubmitReview(f.ctx, f.actor, stale)
	requireReviewError(t, err, problemreference.ErrSourceChanged)

	other := f.actor
	other.SubjectID = identityID(1101)
	f.role(t, other.SubjectID, 0, true)
	_, err = f.store.SubmitReview(f.ctx, other, intent)
	requireReviewError(t, err, problemreference.ErrNotFound)

	type result struct {
		receipt problemreference.ReviewReceipt
		err     error
	}
	start, results := make(chan struct{}), make(chan result, 2)
	for range 2 {
		go func() {
			<-start
			r, err := f.store.SubmitReview(f.ctx, f.actor, intent)
			results <- result{r, err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	must(t, first.err)
	must(t, second.err)
	if !first.receipt.Valid() || first.receipt != second.receipt {
		t.Fatal("same-key submissions did not receive the original receipt")
	}
	got, err := f.store.ReviewReceipt(f.ctx, f.actor, intent.RequestKey)
	must(t, err)
	if got != first.receipt || got.ContentVersion != f.draft.Version {
		t.Fatal("receipt did not identify the exact submitted draft")
	}
	_, err = f.store.ReviewReceipt(f.ctx, other, intent.RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)

	var rows int
	var verdict, diff, lock, plan, profile, evidence string
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM problem_review_submissions`).Scan(&rows))
	must(t, f.a.QueryRow(f.ctx, `SELECT formal_verdict,differential_outcome,source_lock_sha256,
		formal_plan_sha256,profile_sha256,evidence_sha256 FROM problem_review_submissions WHERE id=$1`, got.ID).
		Scan(&verdict, &diff, &lock, &plan, &profile, &evidence))
	digest := sha256.Sum256(report.Evidence)
	if rows != 1 || verdict != "WA" || diff != "" || lock != f.request.Specification.Identity.SourceLockSHA256 ||
		plan != f.want.SHA256 || profile != f.request.Specification.ProfileSHA256 || evidence != hex.EncodeToString(digest[:]) {
		t.Fatal("intake did not preserve the server-derived evidence")
	}
	draft, err := f.repository.GetOwn(f.ctx, f.actor, f.draft.ID)
	must(t, err)
	if !reflect.DeepEqual(draft, f.draft) {
		t.Fatal("intake changed the draft")
	}
	f.role(t, f.actor.SubjectID, 1, false)
	_, err = f.store.ReviewReceipt(f.ctx, f.actor, intent.RequestKey)
	requireReviewError(t, err, identity.ErrForbidden)
	_, err = f.store.SubmitReview(f.ctx, f.actor, intent)
	requireReviewError(t, err, identity.ErrForbidden)
	f.role(t, f.actor.SubjectID, 2, true)
	regranted, err := f.store.ReviewReceipt(f.ctx, f.actor, intent.RequestKey)
	must(t, err)
	if regranted != got {
		t.Fatal("regrant rewrote the original intake record")
	}
	if _, err := f.provider.DownTo(f.ctx, 14); err == nil || !strings.Contains(err.Error(), "cannot discard review submissions") {
		t.Fatalf("nonempty rollback must refuse to delete intake records: %v", err)
	}
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM problem_review_submissions`).Scan(&rows))
	if rows != 1 {
		t.Fatal("rollback changed the ledger")
	}
}

func TestReviewIntakeRecoversLostCommit(t *testing.T) {
	f, claims, node, report := acceptedReportFixture(t, "AC")
	must(t, claims.ReportReference(f.ctx, node, report))
	fault, observed := lostReferenceCommitStore(t, f.referenceRequestFixture)
	intent := reviewIntent(f)
	got, err := fault.SubmitReview(f.ctx, f.actor, intent)
	requireReviewError(t, err, problemreference.ErrCommitUnknown)
	if got != (problemreference.ReviewReceipt{}) {
		t.Fatal("uncertain commit exposed a receipt")
	}
	select {
	case committed := <-observed:
		if !committed {
			t.Fatal("did not lose an actual COMMIT reply")
		}
	case <-f.ctx.Done():
		t.Fatal("commit-loss fixture timed out")
	}
	receipt, err := f.store.ReviewReceipt(f.ctx, f.actor, intent.RequestKey)
	must(t, err)
	replay, err := f.store.SubmitReview(f.ctx, f.actor, intent)
	must(t, err)
	if !receipt.Valid() || receipt != replay {
		t.Fatal("lost reply did not recover the exact original receipt")
	}

	// A second accepted report is valid for this same draft but cannot
	// change the evidence bound to the original key.
	const secondKey = "review-second-reference-key"
	second, err := f.store.Enqueue(f.ctx, f.actor, secondKey, f.draft.ID, f.catalog)
	must(t, err)
	prepareClaim(t, f, secondKey)
	claim, err := claims.ClaimReference(f.ctx, node, problemreference.ClaimInput{
		RequestID: second.ID, SourceID: f.draft.SourceID, PlanSHA256: f.want.SHA256})
	must(t, err)
	must(t, claims.ReportReference(f.ctx, node, workerReportForClaim(t, f, claim, "WA")))
	intent.ReferenceRequestID = second.ID
	_, err = f.store.SubmitReview(f.ctx, f.actor, intent)
	requireReviewError(t, err, problemreference.ErrConflict)
	still, err := f.store.ReviewReceipt(f.ctx, f.actor, intent.RequestKey)
	must(t, err)
	if still != receipt {
		t.Fatal("conflicting evidence changed the original receipt")
	}
	intent.RequestKey = "review-intake-distinct-key"
	distinct, err := f.store.SubmitReview(f.ctx, f.actor, intent)
	must(t, err)
	if distinct.ID == receipt.ID || distinct.ReviewIntent != intent {
		t.Fatal("different key did not retain its own evidence")
	}

	assertReviewLookupPlan(t, f, receipt)
}

func TestReviewIntakeRejectsUnboundEvidence(t *testing.T) {
	f, claims, node, report := acceptedReportFixture(t, "AC")
	must(t, claims.ReportReference(f.ctx, node, report))
	wrongProfile := bytes.Replace(report.Evidence, []byte(f.request.Specification.ProfileSHA256), []byte(strings.Repeat("0", 64)), 1)
	if bytes.Equal(wrongProfile, report.Evidence) {
		t.Fatal("missing profile in report fixture")
	}
	_, err := f.a.Exec(f.ctx, `UPDATE problem_reference_worker_reports
		SET evidence=$1, evidence_sha256=encode(sha256($1),'hex') WHERE request_id=$2`, wrongProfile, f.request.ID)
	must(t, err)
	got, err := f.store.SubmitReview(f.ctx, f.actor, reviewIntent(f))
	if err == nil || got != (problemreference.ReviewReceipt{}) {
		t.Fatal("intake accepted evidence bound to a different profile")
	}
	_, err = f.store.ReviewReceipt(f.ctx, f.actor, reviewIntent(f).RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)
}

func assertReviewLookupPlan(t *testing.T, f expectedPlanFixture, receipt problemreference.ReviewReceipt) {
	t.Helper()
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_submissions
		(id,owner_subject_id,request_key,draft_id,source_id,content_version,import_revision,source_lock_sha256,
		reference_request_id,formal_plan_sha256,profile_sha256,evidence_sha256,formal_verdict,
		differential_plan_sha256,differential_outcome,auth_generation,recovery_id,author_revision)
		SELECT md5('review-plan-'||n::text),owner_subject_id,'review-plan-lookup-'||n::text,draft_id,source_id,
		content_version,import_revision,source_lock_sha256,reference_request_id,formal_plan_sha256,
		profile_sha256,evidence_sha256,formal_verdict,differential_plan_sha256,differential_outcome,
		auth_generation,recovery_id,author_revision
		FROM problem_review_submissions CROSS JOIN generate_series(1,2048) n WHERE id=$1`, receipt.ID)
	must(t, err)
	_, err = tx.Exec(f.ctx, `ANALYZE problem_review_submissions`)
	must(t, err)
	sql, err := os.ReadFile("../../db/queries/problemreference/review_intake.sql")
	must(t, err)
	_, query, found := strings.Cut(string(sql), "-- name: ReviewSubmissionByOwnerKey :one\n")
	if !found {
		t.Fatal("missing owner/key query")
	}
	query = strings.NewReplacer("@owner", "$1", "@request_key", "$2").Replace(query)
	var plan string
	must(t, tx.QueryRow(f.ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+query, receipt.Owner, receipt.RequestKey).Scan(&plan))
	if !strings.Contains(plan, "problem_review_submissions_owner_subject_id_request_key_key") ||
		!strings.Contains(plan, "Index Cond") || !strings.Contains(plan, `"Actual Rows": 1`) {
		t.Fatal("receipt did not use its unique owner/key index", plan)
	}
	t.Log(plan)
}

func reviewIntent(f expectedPlanFixture) problemreference.ReviewIntent {
	return problemreference.ReviewIntent{RequestKey: "review-intake-original-key", DraftID: f.draft.ID,
		ImportRevision: f.draft.ImportRevision, ReferenceRequestID: f.request.ID}
}

func requireReviewError(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("review error=%v, want %v", err, want)
	}
}

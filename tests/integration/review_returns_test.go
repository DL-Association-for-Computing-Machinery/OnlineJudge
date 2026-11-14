//go:build integration

package integration_test

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
)

func returnIntent(e problemreference.ReviewEvidence, key string) problemreference.ReturnIntent {
	return problemreference.ReturnIntent{RequestKey: key, AssignmentRevision: e.AssignmentRevision,
		ImportRevision: e.ImportRevision, EvidenceSHA256: e.EvidenceSHA256, Reason: "请补齐最大规模样例。"}
}

func assignReview(t *testing.T, f reviewFixture, id string) problemreference.ReviewEvidence {
	t.Helper()
	c := f.change(0, f.reader.SubjectID, 1)
	c.ReviewID = id
	_, err := f.reviews.SetAssignment(f.ctx, c)
	must(t, err)
	e, err := f.reviews.ReviewEvidence(f.ctx, f.reader, id)
	must(t, err)
	return e
}

func TestReviewReturnAppliesToDuplicateMaterialAndPreservesOriginalKeys(t *testing.T) {
	f := newReviewFixture(t)
	i := reviewIntent(f.expectedPlanFixture)
	duplicate := i
	duplicate.RequestKey = "duplicate-review-original-key"
	duplicateReceipt, err := f.store.SubmitReview(f.ctx, f.actor, duplicate)
	must(t, err)
	f.grant(t, f.actor, 0, true)
	f.grant(t, f.reader, 0, true)
	f.role(t, f.reader.SubjectID, 0, true)
	e := assignReview(t, f, f.item.ID)
	other := f
	other.reader.SubjectID = identityID(1102)
	h := identityHasher(t, false)
	_, err = f.identities.Initialize(f.ctx, other.reader.SubjectID, true, identityRecord(t, h, "synthetic second reviewer password"))
	must(t, err)
	f.grant(t, other.reader, 0, true)
	d := assignReview(t, other, duplicateReceipt.ID)
	for _, evidence := range []problemreference.ReviewEvidence{e, d} {
		actor := f.reader
		actor.SubjectID = evidence.Reviewer
		snapshot, err := f.reviews.ReviewEligibilitySnapshot(f.ctx, actor, evidence.ReviewID)
		must(t, err)
		if snapshot.Returned || snapshot.Evidence != evidence {
			t.Fatal("newly assigned item did not preserve its evidence")
		}
	}
	initial := returnIntent(e, "return-original-material-key")
	_, err = f.reviews.ReturnReview(f.ctx, f.actor, f.item.ID, initial)
	requireReviewError(t, err, problemreference.ErrNotFound)
	stale := initial
	stale.AssignmentRevision++
	_, err = f.reviews.ReturnReview(f.ctx, f.reader, f.item.ID, stale)
	requireReviewError(t, err, problemreference.ErrReviewChanged)

	type result struct {
		r   problemreference.ReviewReturnReceipt
		err error
	}
	results := make(chan result, 2)
	for _, item := range []problemreference.ReviewEvidence{e, d} {
		go func() {
			key := "return-" + item.ReviewID
			actor := f.reader
			actor.SubjectID = item.Reviewer
			r, err := f.reviews.ReturnReview(f.ctx, actor, item.ReviewID, returnIntent(item, key))
			results <- result{r, err}
		}()
	}
	var winner problemreference.ReviewReturnReceipt
	var returned int
	for range 2 {
		r := <-results
		if r.err == nil {
			winner = r.r
		} else if errors.Is(r.err, problemreference.ErrReviewReturned) {
			returned++
		} else {
			t.Fatal(r.err)
		}
	}
	if !winner.Valid() || returned != 1 {
		t.Fatal("expected exactly one return for the same immutable material")
	}
	for _, evidence := range []problemreference.ReviewEvidence{e, d} {
		actor := f.reader
		actor.SubjectID = evidence.Reviewer
		snapshot, err := f.reviews.ReviewEligibilitySnapshot(f.ctx, actor, evidence.ReviewID)
		must(t, err)
		if !snapshot.Returned || snapshot.Evidence != evidence {
			t.Fatal("return did not block both original and duplicate material")
		}
	}
	winnerActor := f.reader
	winnerActor.SubjectID = winner.Reviewer
	same, err := f.reviews.ReturnReview(f.ctx, winnerActor, winner.ReviewID, winner.ReturnIntent)
	must(t, err)
	if same != winner {
		t.Fatal("exact reviewer retry changed the original receipt")
	}
	different := winner.ReturnIntent
	different.Reason = "A changed reason"
	_, err = f.reviews.ReturnReview(f.ctx, winnerActor, winner.ReviewID, different)
	requireReviewError(t, err, problemreference.ErrConflict)
	for _, pair := range []struct {
		i    problemreference.ReviewIntent
		want problemreference.ReviewReceipt
	}{{i, f.item}, {duplicate, duplicateReceipt}} {
		status, err := f.store.AuthorReviewReturn(f.ctx, f.actor, pair.i.RequestKey)
		must(t, err)
		if status.ReviewID != pair.want.ID || status.Reason != winner.Reason || status.ReturnedAt != winner.ReturnedAt {
			t.Fatal("author duplicate did not see the same return")
		}
		old, err := f.store.SubmitReview(f.ctx, f.actor, pair.i)
		must(t, err)
		if old != pair.want {
			t.Fatal("return changed an original author receipt")
		}
	}
	i.RequestKey = "review-retry-with-new-key"
	_, err = f.store.SubmitReview(f.ctx, f.actor, i)
	requireReviewError(t, err, problemreference.ErrReviewReturned)
	_, err = f.store.AuthorReviewReturn(f.ctx, f.reader, duplicate.RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)
	f.grant(t, winnerActor, 1, false)
	_, err = f.reviews.ReviewReturnReceipt(f.ctx, winnerActor, winner.ReviewID, winner.RequestKey)
	if err == nil {
		t.Fatal("revoked reviewer recovered private receipt")
	}
	if _, err := f.provider.DownTo(f.ctx, 16); err == nil || !strings.Contains(err.Error(), "cannot discard review returns") {
		t.Fatalf("rollback discarded a nonempty decision history: %v", err)
	}
	// Down18 succeeds on the empty confirmation ledger before Down17 is refused.
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
}

func TestReviewReturnRecoversRealLostCommit(t *testing.T) {
	f := newReviewFixture(t)
	f.grant(t, f.reader, 0, true)
	e := assignReview(t, f, f.item.ID)
	i := returnIntent(e, "review-return-unknown-key")
	pool, observed := lostReferenceCommitPool(t, f.referenceRequestFixture)
	fault, err := referencepg.NewReviewStore(pool, f.access)
	must(t, err)
	r, err := fault.ReturnReview(f.ctx, f.reader, e.ReviewID, i)
	requireReviewError(t, err, problemreference.ErrCommitUnknown)
	if r != (problemreference.ReviewReturnReceipt{}) {
		t.Fatal("unconfirmed return disclosed a receipt")
	}
	select {
	case committed := <-observed:
		if !committed {
			t.Fatal("did not lose a real PostgreSQL COMMIT reply")
		}
	case <-f.ctx.Done():
		t.Fatal("COMMIT fixture did not run")
	}
	r, err = f.reviews.ReviewReturnReceipt(f.ctx, f.reader, e.ReviewID, i.RequestKey)
	must(t, err)
	replayed, err := f.reviews.ReturnReview(f.ctx, f.reader, e.ReviewID, i)
	must(t, err)
	if !r.Valid() || r != replayed || r.ReturnIntent != i {
		t.Fatal("original key did not recover the exact return")
	}
	var count int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM problem_review_returns`).Scan(&count))
	if count != 1 {
		t.Fatal("recovery wrote another return")
	}
	assertReviewReturnPlans(t, f, r)
}

func assertReviewReturnPlans(t *testing.T, f reviewFixture, r problemreference.ReviewReturnReceipt) {
	t.Helper()
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_submissions
		(id,owner_subject_id,request_key,draft_id,source_id,content_version,import_revision,source_lock_sha256,
		reference_request_id,formal_plan_sha256,profile_sha256,evidence_sha256,formal_verdict,
		differential_plan_sha256,differential_outcome,auth_generation,recovery_id,author_revision)
		SELECT md5('return-plan-'||n::text),owner_subject_id,'return-plan-intake-'||n::text,draft_id,source_id,
		content_version,import_revision,source_lock_sha256,reference_request_id,formal_plan_sha256,
		profile_sha256,evidence_sha256,formal_verdict,differential_plan_sha256,differential_outcome,
		auth_generation,recovery_id,author_revision
		FROM problem_review_submissions CROSS JOIN generate_series(1,2048) n WHERE id=$1`, r.ReviewID)
	must(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_returns
		(review_id,material_sha256,reviewer_subject_id,request_key,reason,auth_generation,recovery_id,reviewer_revision,assignment_revision)
		SELECT s.id,encode(sha256(convert_to(s.id,'UTF8')),'hex'),r.reviewer_subject_id,s.request_key,r.reason,
		r.auth_generation,r.recovery_id,r.reviewer_revision,r.assignment_revision
		FROM problem_review_submissions s CROSS JOIN problem_review_returns r
		WHERE r.review_id=$1 AND s.request_key LIKE 'return-plan-intake-%'`, r.ReviewID)
	must(t, err)
	_, err = tx.Exec(f.ctx, `ANALYZE problem_review_returns`)
	must(t, err)
	var material string
	must(t, tx.QueryRow(f.ctx, `SELECT material_sha256 FROM problem_review_returns WHERE review_id=$1`, r.ReviewID).Scan(&material))
	raw, err := os.ReadFile("../../db/queries/problemreference/review_returns.sql")
	must(t, err)
	for _, tc := range []struct {
		name, index string
		params      *strings.Replacer
		args        []any
	}{
		{"ReviewReturnByMaterial", "problem_review_returns_material_sha256_key", strings.NewReplacer("@material_sha256", "$1"), []any{material}},
		{"ReviewReturnByReviewerKey", "problem_review_returns_reviewer_subject_id_request_key_key",
			strings.NewReplacer("@reviewer_subject_id", "$1", "@request_key", "$2"), []any{r.Reviewer, r.RequestKey}},
	} {
		_, tail, ok := strings.Cut(string(raw), "-- name: "+tc.name+" :one\n")
		if !ok {
			t.Fatal("missing authoritative return query")
		}
		sql, _, _ := strings.Cut(tail, "\n-- name:")
		var plan string
		must(t, tx.QueryRow(f.ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+tc.params.Replace(sql), tc.args...).Scan(&plan))
		if !strings.Contains(plan, tc.index) || !strings.Contains(plan, `"Actual Rows": 1`) {
			t.Fatal("return lookup lost its bounded index plan", plan)
		}
		t.Log(tc.name, plan)
	}
}

func TestReviewReturnSeesWithdrawalAfterItemLock(t *testing.T) {
	f := newReviewFixture(t)
	f.grant(t, f.reader, 0, true)
	e := assignReview(t, f, f.item.ID)
	i := returnIntent(e, "review-return-wait-withdrawal")
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	var holder uint32
	var id string
	must(t, tx.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&holder))
	must(t, tx.QueryRow(f.ctx, `SELECT id FROM problem_review_submissions WHERE id=$1 FOR UPDATE`, f.item.ID).Scan(&id))
	done := make(chan error, 1)
	go func() {
		_, err := f.reviews.ReturnReview(f.ctx, f.reader, id, i)
		done <- err
	}()
	waitBacklogBlocked(t, f.backlogFixture, holder, f.appB)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_assignment_history
		(review_id,revision,maintenance_operator,reason) VALUES($1,2,'fixture','withdraw')`, id)
	must(t, err)
	must(t, tx.Commit(f.ctx))
	requireReviewError(t, <-done, problemreference.ErrNotFound)
	_, err = f.store.AuthorReviewReturn(f.ctx, f.actor, f.item.RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)
}

func TestReviewReturnIntakeWaitsAndSeesCommittedReturn(t *testing.T) {
	f := newReviewFixture(t)
	f.grant(t, f.reader, 0, true)
	e := assignReview(t, f, f.item.ID)
	i := returnIntent(e, "review-return-held-commit")
	digest := sha256.Sum256([]byte("onlinejudge/review-material/v1:" + f.actor.SubjectID +
		e.ImportRevision + e.FormalPlanSHA256 + e.ProfileSHA256 + e.EvidenceSHA256 + e.ComparisonPlan))
	tx, err := f.b.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	var holder uint32
	must(t, tx.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&holder))
	_, err = tx.Exec(f.ctx, `SELECT pg_advisory_xact_lock($1::bigint)`, int64(binary.BigEndian.Uint64(digest[:8])))
	must(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_returns
		(review_id,material_sha256,reviewer_subject_id,request_key,reason,auth_generation,recovery_id,reviewer_revision,assignment_revision)
		VALUES($1,$2,$3,$4,$5,$6,$7,1,$8)`, f.item.ID, hex.EncodeToString(digest[:]), f.reader.SubjectID,
		i.RequestKey, i.Reason, f.reader.AuthGeneration, f.reader.RecoveryID, i.AssignmentRevision)
	must(t, err)
	intake := reviewIntent(f.expectedPlanFixture)
	intake.RequestKey = "new-intake-while-return-held"
	done := make(chan error, 1)
	go func() {
		_, err := f.store.SubmitReview(f.ctx, f.actor, intake)
		done <- err
	}()
	waitBacklogBlocked(t, f.backlogFixture, holder, f.appA)
	must(t, tx.Commit(f.ctx))
	requireReviewError(t, <-done, problemreference.ErrReviewReturned)
	_, err = f.store.ReviewReceipt(f.ctx, f.actor, intake.RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)
}

func TestReviewEligibilityWaitsForTheSameMaterialDecision(t *testing.T) {
	f := newReviewFixture(t)
	f.grant(t, f.reader, 0, true)
	e := assignReview(t, f, f.item.ID)
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	var holder uint32
	must(t, tx.QueryRow(f.ctx, "SELECT pg_backend_pid()").Scan(&holder))
	digest := sha256.Sum256([]byte("onlinejudge/review-material/v1:" + f.actor.SubjectID +
		e.ImportRevision + e.FormalPlanSHA256 + e.ProfileSHA256 + e.EvidenceSHA256 + e.ComparisonPlan))
	_, err = tx.Exec(f.ctx, "SELECT pg_advisory_xact_lock($1::bigint)", int64(binary.BigEndian.Uint64(digest[:8])))
	must(t, err)
	type result struct {
		snapshot problemreference.ReviewEligibilitySnapshot
		err      error
	}
	done := make(chan result, 1)
	go func() {
		snapshot, err := f.reviews.ReviewEligibilitySnapshot(f.ctx, f.reader, e.ReviewID)
		done <- result{snapshot, err}
	}()
	waitBacklogBlocked(t, f.backlogFixture, holder, f.appB)
	i := returnIntent(e, "review-return-waits-eligibility")
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_returns
		(review_id,material_sha256,reviewer_subject_id,request_key,reason,auth_generation,recovery_id,reviewer_revision,assignment_revision)
		VALUES($1,$2,$3,$4,$5,$6,$7,1,$8)`, e.ReviewID, hex.EncodeToString(digest[:]), f.reader.SubjectID,
		i.RequestKey, i.Reason, f.reader.AuthGeneration, f.reader.RecoveryID, i.AssignmentRevision)
	must(t, err)
	must(t, tx.Commit(f.ctx))
	got := <-done
	must(t, got.err)
	if !got.snapshot.Returned || got.snapshot.Evidence != e {
		t.Fatal("eligibility returned an observation from before the committed material decision")
	}
}

func TestReviewReturnCoversDuplicateIntakeCommittedFirst(t *testing.T) {
	f := newReviewFixture(t)
	f.grant(t, f.reader, 0, true)
	e := assignReview(t, f, f.item.ID)
	digest := sha256.Sum256([]byte("onlinejudge/review-material/v1:" + f.actor.SubjectID +
		e.ImportRevision + e.FormalPlanSHA256 + e.ProfileSHA256 + e.EvidenceSHA256 + e.ComparisonPlan))
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	var holder uint32
	must(t, tx.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&holder))
	_, err = tx.Exec(f.ctx, `SELECT pg_advisory_xact_lock($1::bigint)`, int64(binary.BigEndian.Uint64(digest[:8])))
	must(t, err)
	const duplicateKey = "duplicate-intake-held-commit"
	duplicateID := identityID(1201)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_submissions
		(id,owner_subject_id,request_key,draft_id,source_id,content_version,import_revision,source_lock_sha256,
		reference_request_id,formal_plan_sha256,profile_sha256,evidence_sha256,formal_verdict,
		differential_plan_sha256,differential_outcome,auth_generation,recovery_id,author_revision)
		SELECT $2,owner_subject_id,$3,draft_id,source_id,content_version,import_revision,source_lock_sha256,
		reference_request_id,formal_plan_sha256,profile_sha256,evidence_sha256,formal_verdict,
		differential_plan_sha256,differential_outcome,auth_generation,recovery_id,author_revision
		FROM problem_review_submissions WHERE id=$1`, f.item.ID, duplicateID, duplicateKey)
	must(t, err)
	i := returnIntent(e, "review-return-wait-intake")
	done := make(chan error, 1)
	go func() {
		_, err := f.reviews.ReturnReview(f.ctx, f.reader, e.ReviewID, i)
		done <- err
	}()
	waitBacklogBlocked(t, f.backlogFixture, holder, f.appB)
	must(t, tx.Commit(f.ctx))
	must(t, <-done)
	got, err := f.store.AuthorReviewReturn(f.ctx, f.actor, duplicateKey)
	must(t, err)
	if got.ReviewID != duplicateID || got.Reason != i.Reason {
		t.Fatal("intake committed first did not acquire the same material-level return")
	}
}

func TestReviewReturnAllowsNewAcceptedEvidence(t *testing.T) {
	f, claims, node, report := acceptedReportFixture(t, "WA")
	must(t, claims.ReportReference(f.ctx, node, report))
	intent := reviewIntent(f)
	item, err := f.store.SubmitReview(f.ctx, f.actor, intent)
	must(t, err)
	r, err := referencepg.NewReviewStore(f.b, f.access)
	must(t, err)
	reader := f.actor
	reader.SubjectID = identityID(1101)
	fixture := reviewFixture{expectedPlanFixture: f, reviews: r, item: item, reader: reader}
	fixture.grant(t, reader, 0, true)
	e := assignReview(t, fixture, item.ID)
	_, err = r.ReturnReview(f.ctx, reader, item.ID, returnIntent(e, "return-old-evidence-key"))
	must(t, err)

	const next = "review-after-return-reference"
	second, err := f.store.Enqueue(f.ctx, f.actor, next, f.draft.ID, f.catalog)
	must(t, err)
	prepareClaim(t, f, next)
	claim, err := claims.ClaimReference(f.ctx, node, problemreference.ClaimInput{
		RequestID: second.ID, SourceID: f.draft.SourceID, PlanSHA256: f.want.SHA256})
	must(t, err)
	must(t, claims.ReportReference(f.ctx, node, workerReportForClaim(t, f, claim, "AC")))
	intent.RequestKey, intent.ReferenceRequestID = "review-changed-evidence-key", second.ID
	got, err := f.store.SubmitReview(f.ctx, f.actor, intent)
	must(t, err)
	if !got.Valid() || got.ImportRevision != item.ImportRevision || got.ReferenceRequestID == item.ReferenceRequestID {
		t.Fatal("updated evidence did not create a distinct intake")
	}
	_, err = f.store.AuthorReviewReturn(f.ctx, f.actor, intent.RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)
}

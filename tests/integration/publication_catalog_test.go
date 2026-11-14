//go:build integration && linux

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/jackc/pgx/v5/pgconn"
)

type missingPublicationObjects struct{}

func (missingPublicationObjects) Open(context.Context, materialstore.Object) (*materialstore.Snapshot, error) {
	return nil, errors.New("fixture object missing")
}

func TestPublicationCatalogLocalChain(t *testing.T) {
	f, e := publicationApprovalFixture(t)
	_, err := f.provider.DownTo(f.ctx, 28)
	must(t, err)
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	_, err = f.access.SetPublisherRole(f.ctx, publisherChange(f.actor.SubjectID, 0, true))
	must(t, err)
	draft, err := f.repository.GetOwn(f.ctx, f.actor, e.DraftID)
	must(t, err)
	bound, err := draft.MaterialInventoryIdentity()
	must(t, err)
	var plan problem.ReferencePlan
	must(t, json.Unmarshal(f.want.Encoding, &plan))
	publicID := identityID(5511)
	prepared, err := draft.PlanPublicationMaterials(publicID, e.DraftID, f.actor.SubjectID, bound, plan, e.FormalPlanSHA256)
	must(t, err)
	sealed, err := prepared.Seal(f.ctx, f.materials)
	must(t, err)
	display, err := problem.ReadSealedPublicationStatement(f.ctx, f.materials, sealed)
	must(t, err)

	intent := referencepg.PublicationIntent{Materials: sealed, ReviewID: e.ReviewID, ReviewerID: e.Reviewer,
		AssignmentRevision: e.AssignmentRevision, PublisherRevision: 1, RequestKey: "local-publication-chain",
		ExecutionVersion: "local-catalog-fixture", ExecutionSHA256: strings.Repeat("a", 64)}
	_, err = f.reviews.PublishProblem(f.ctx, f.access, f.reader, f.materials, plan, intent)
	requireReviewError(t, err, identity.ErrPublisherRequired)
	_, err = f.reviews.PublishProblem(f.ctx, f.access, f.actor, f.materials, plan, intent)
	requireReviewError(t, err, problemreference.ErrNotFound)
	snapshot := currentPublicationSnapshot(t, f, e)
	approval := problemreference.ApprovalIntent{RequestKey: "local-approval-chain", AssignmentRevision: e.AssignmentRevision,
		SnapshotSHA256: snapshot, Reason: "isolated approval chain fixture"}
	_, err = f.reviews.ApproveReview(f.ctx, f.reader, e.ReviewID, approval)
	requireReviewError(t, err, problemreference.ErrReviewApprovalBlocked)
	_, err = f.reviews.ReviewApprovalReceipt(f.ctx, f.reader, e.ReviewID, approval.RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)
	seedPublicationApproval(t, f, e, snapshot, problemreference.ReviewPolicyVersion)
	_, err = f.reviews.PublishProblem(f.ctx, f.access, f.actor, f.materials, plan, intent)
	requireReviewError(t, err, problemreference.ErrReviewApprovalBlocked)
	page, next, err := f.reviews.ListPublished(f.ctx, "", "")
	must(t, err)
	if len(page) != 0 || next != "" {
		t.Fatal("failed publication made the draft visible")
	}
	_, err = f.reviews.ReadPublished(f.ctx, f.materials, publicID)
	requireReviewError(t, err, problemreference.ErrNotFound)

	// Isolated historical fixtures to exercise catalog reads. The real writer
	// above remains blocked, and ordinary runtime has no admission writer.
	_, err = f.a.Exec(f.ctx, `INSERT INTO submission_materials (version_id, manifest_sha256, manifest)
		VALUES ($1,$2,'{}')`, intent.ExecutionVersion, intent.ExecutionSHA256)
	must(t, err)
	const insert = `INSERT INTO problem_publications
		(problem_id,revision,version,title,statement_sha256,statement_size,import_revision,formal_plan_sha256,
		execution_version,execution_sha256,review_id,assignment_revision,publisher_id,publisher_revision,request_key)
		VALUES ($1,1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,1,$13)`
	args := []any{publicID, sealed.Version, display.Title, sealed.Statement.SHA256, sealed.Statement.Size, bound.ImportRevision,
		e.FormalPlanSHA256, intent.ExecutionVersion, intent.ExecutionSHA256, e.ReviewID, e.AssignmentRevision,
		f.actor.SubjectID, intent.RequestKey}
	_, err = f.a.Exec(f.ctx, insert, args...)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("execution registration alone must not satisfy publication: %v", err)
	}
	_, err = f.a.Exec(f.ctx, `INSERT INTO problem_execution_admissions
		(import_revision, formal_plan_sha256, execution_version, execution_sha256) VALUES ($1,$2,$3,$4)`,
		bound.ImportRevision, e.FormalPlanSHA256, intent.ExecutionVersion, intent.ExecutionSHA256)
	must(t, err)
	_, err = f.a.Exec(f.ctx, insert, args...)
	must(t, err)
	replay, err := f.reviews.PublishProblem(f.ctx, f.access, f.actor, f.materials, plan, intent)
	must(t, err)
	if replay.ID != publicID || replay.Version != sealed.Version {
		t.Fatal("original key did not return the recorded publication")
	}
	changed := intent
	changed.ExpectedRevision = 1
	_, err = f.reviews.PublishProblem(f.ctx, f.access, f.actor, f.materials, plan, changed)
	requireReviewError(t, err, problemreference.ErrConflict)

	f.grant(t, f.reader, 1, false)
	got, err := f.reviews.ReadPublished(f.ctx, f.materials, publicID)
	must(t, err)
	gotJSON, err := json.Marshal(got)
	must(t, err)
	wantJSON, err := json.Marshal(display)
	must(t, err)
	if string(gotJSON) != string(wantJSON) || strings.Contains(string(gotJSON), e.DraftID) ||
		strings.Contains(string(gotJSON), e.FormalPlanSHA256) || strings.Contains(string(gotJSON), "synthetic expected plan source") {
		t.Fatal("catalog exposed more than the sealed display")
	}
	missing, err := f.reviews.ReadPublished(f.ctx, missingPublicationObjects{}, publicID)
	requireReviewError(t, err, problemreference.ErrStore)
	if missing.ProblemID != "" || missing.Markdown != "" {
		t.Fatal("missing object returned part of the statement")
	}
	_, err = f.reviews.ReadPublished(f.ctx, f.materials, e.DraftID)
	requireReviewError(t, err, problemreference.ErrNotFound)
	page, next, err = f.reviews.ListPublished(f.ctx, "", strings.ToUpper(display.Title))
	must(t, err)
	if len(page) != 1 || page[0].ID != publicID || page[0].Version != sealed.Version || next != "" {
		t.Fatal("catalog search did not find the published projection")
	}
	for _, filter := range []struct{ after, query string }{{publicID, ""}, {"", "%"}, {"", e.DraftID}} {
		page, next, err = f.reviews.ListPublished(f.ctx, filter.after, filter.query)
		must(t, err)
		if len(page) != 0 || next != "" {
			t.Fatal("catalog returned results for an unmatched search or exhausted cursor")
		}
	}
	// Only pagination and latest-row selection are under test here. These
	// synthetic catalog rows deliberately do not open any additional objects.
	_, err = f.a.Exec(f.ctx, `WITH original AS (
		SELECT * FROM problem_publications WHERE problem_id=$1 AND revision=1
	), extra AS (
		INSERT INTO problem_review_submissions
		SELECT lpad(to_hex(n),32,'0'), r.owner_subject_id, 'page-review-key-'||n, r.draft_id, r.source_id, r.content_version,
			r.import_revision, r.source_lock_sha256, r.reference_request_id, r.formal_plan_sha256, r.profile_sha256, r.evidence_sha256,
			r.formal_verdict, r.differential_plan_sha256, r.differential_outcome, r.auth_generation, r.recovery_id, r.author_revision, now()
		FROM problem_review_submissions r JOIN original o ON o.review_id=r.id CROSS JOIN generate_series(1,22) n
		RETURNING id
	), assignments AS (
		INSERT INTO problem_review_assignment_history (review_id,revision,reviewer_subject_id,reviewer_revision,maintenance_operator,reason)
		SELECT id,1,$2,1,'fixture','catalog pagination' FROM extra RETURNING review_id
	), approvals AS (
		INSERT INTO problem_review_approvals
			(review_id,assignment_revision,reviewer_subject_id,reviewer_revision,request_key,material_sha256,snapshot_sha256,
			policy_version,reason,auth_generation,recovery_id)
		SELECT a.review_id,1,$2,1,'page-approve-'||a.review_id,repeat('b',64),repeat('c',64),'fixture','fixture',1,$3
		FROM assignments a RETURNING review_id
	), numbered AS (
		SELECT review_id, row_number() OVER (ORDER BY review_id) n FROM approvals
	)
	INSERT INTO problem_publications
		(problem_id,revision,version,title,statement_sha256,statement_size,import_revision,formal_plan_sha256,execution_version,
		execution_sha256,review_id,assignment_revision,publisher_id,publisher_revision,request_key)
	SELECT CASE WHEN a.n=22 THEN o.problem_id ELSE a.review_id END, CASE WHEN a.n=22 THEN 2 ELSE 1 END,
		repeat('d',64),'Latest fixture title',o.statement_sha256,o.statement_size,o.import_revision,o.formal_plan_sha256,o.execution_version,
		o.execution_sha256,a.review_id,1,o.publisher_id,1,'page-publish-'||a.review_id
	FROM numbered a CROSS JOIN original o`, publicID, e.Reviewer, f.reader.RecoveryID)
	must(t, err)
	page, next, err = f.reviews.ListPublished(f.ctx, "", "Latest fixture title")
	must(t, err)
	if len(page) != 20 || next != page[19].ID {
		t.Fatal("catalog pagination did not return a full first page")
	}
	page, next, err = f.reviews.ListPublished(f.ctx, next, "Latest fixture title")
	must(t, err)
	if len(page) != 2 || page[1].ID != publicID || next != "" {
		t.Fatal("catalog page skipped the latest revision or repeated an ID")
	}
	page, _, err = f.reviews.ListPublished(f.ctx, "", display.Title)
	must(t, err)
	if len(page) != 0 {
		t.Fatal("catalog search revived an old title")
	}
	var lookupPlan string
	must(t, f.a.QueryRow(f.ctx, `EXPLAIN (FORMAT JSON) SELECT * FROM problem_publications
		WHERE problem_id=$1 ORDER BY revision DESC LIMIT 1`, publicID).Scan(&lookupPlan))
	if !strings.Contains(lookupPlan, "problem_publications_pkey") {
		t.Fatalf("latest revision lookup did not use the composite primary key: %s", lookupPlan)
	}
	_, err = f.provider.DownTo(f.ctx, 28)
	if err == nil || !strings.Contains(err.Error(), "cannot discard publication history") {
		t.Fatalf("rollback did not preserve publication history: %v", err)
	}
}

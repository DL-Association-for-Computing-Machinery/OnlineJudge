-- name: ReportedReferenceForReview :one
SELECT * FROM problem_reference_requests
WHERE id = @id AND owner_subject_id = @owner AND draft_id = @draft AND state = 'reported';

-- name: InsertReviewSubmission :one
INSERT INTO problem_review_submissions (
    id, owner_subject_id, request_key, draft_id, source_id, content_version,
    import_revision, source_lock_sha256, reference_request_id, formal_plan_sha256,
    profile_sha256, evidence_sha256, formal_verdict, differential_plan_sha256,
    differential_outcome, auth_generation, recovery_id, author_revision
) VALUES (
    @id, @owner, @request_key, @draft_id, @source_id, @content_version,
    @import_revision, @source_lock_sha256, @reference_request_id, @formal_plan_sha256,
    @profile_sha256, @evidence_sha256, @formal_verdict, @differential_plan_sha256,
    @differential_outcome, @auth_generation, @recovery_id, @author_revision
)
ON CONFLICT (owner_subject_id, request_key) DO NOTHING
RETURNING *;

-- name: ReviewSubmissionByOwnerKey :one
SELECT * FROM problem_review_submissions WHERE owner_subject_id = @owner AND request_key = @request_key;

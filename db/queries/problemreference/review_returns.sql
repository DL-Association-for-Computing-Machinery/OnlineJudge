-- name: LockReviewMaterial :exec
SELECT pg_advisory_xact_lock(@lock_key::bigint);

-- name: ReviewReturnByMaterial :one
SELECT * FROM problem_review_returns WHERE material_sha256 = @material_sha256;

-- name: ReviewReturnByReviewerKey :one
SELECT * FROM problem_review_returns WHERE reviewer_subject_id = @reviewer_subject_id AND request_key = @request_key;

-- name: InsertReviewReturn :one
INSERT INTO problem_review_returns (
    review_id, material_sha256, reviewer_subject_id, request_key, reason,
    auth_generation, recovery_id, reviewer_revision, assignment_revision
) VALUES (
    @review_id, @material_sha256, @reviewer_subject_id, @request_key, @reason,
    @auth_generation, @recovery_id, @reviewer_revision, @assignment_revision
)
ON CONFLICT DO NOTHING
RETURNING *;

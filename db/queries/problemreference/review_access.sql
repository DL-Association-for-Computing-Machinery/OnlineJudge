-- name: ReviewSubmissionForShare :one
SELECT * FROM problem_review_submissions WHERE id = @review_id FOR SHARE;

-- name: ReviewSubmissionForUpdate :one
SELECT * FROM problem_review_submissions WHERE id = @review_id FOR UPDATE;

-- name: LatestReviewAssignment :one
SELECT review_id, revision, reviewer_subject_id, reviewer_revision
FROM problem_review_assignment_history WHERE review_id = @review_id
ORDER BY revision DESC LIMIT 1;

-- name: AppendReviewAssignment :exec
INSERT INTO problem_review_assignment_history (
    review_id, revision, reviewer_subject_id, reviewer_revision, maintenance_operator, reason
) VALUES (@review_id, @revision, sqlc.narg(reviewer_subject_id), sqlc.narg(reviewer_revision), @maintenance_operator, @reason);

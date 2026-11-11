-- name: LatestProblemReviewer :one
SELECT subject_id, revision, enabled
FROM identity_problem_reviewer_history
WHERE subject_id = @subject_id
ORDER BY revision DESC LIMIT 1;

-- name: AppendProblemReviewer :exec
INSERT INTO identity_problem_reviewer_history(subject_id, revision, enabled, maintenance_operator, reason)
VALUES (@subject_id, @revision, @enabled, @maintenance_operator, @reason);

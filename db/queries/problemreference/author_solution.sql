-- name: SolutionConfirmation :one
SELECT * FROM problem_solution_confirmations
WHERE draft_id = @draft_id AND import_revision = @import_revision AND formal_plan_sha256 = @formal_plan_sha256;

-- name: InsertSolutionConfirmation :one
INSERT INTO problem_solution_confirmations (
    draft_id, import_revision, formal_plan_sha256, review_id, owner_subject_id,
    author_revision, auth_generation, recovery_id
) VALUES (@draft_id, @import_revision, @formal_plan_sha256, @review_id, @owner_subject_id,
    @author_revision, @auth_generation, @recovery_id)
ON CONFLICT (draft_id, import_revision, formal_plan_sha256) DO NOTHING
RETURNING *;

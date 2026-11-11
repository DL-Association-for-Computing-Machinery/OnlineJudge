-- name: StatementConfirmationByKey :one
SELECT * FROM problem_statement_confirmations
WHERE reviewer_subject_id = @reviewer_subject_id AND request_key = @request_key;

-- name: StatementConfirmationByAssignment :one
SELECT * FROM problem_statement_confirmations
WHERE review_id = @review_id AND assignment_revision = @assignment_revision;

-- name: InsertStatementConfirmation :one
INSERT INTO problem_statement_confirmations (
    review_id, assignment_revision, reviewer_subject_id, reviewer_revision, request_key,
    import_revision, evidence_sha256, checklist_version, auth_generation, recovery_id
) VALUES (
    @review_id, @assignment_revision, @reviewer_subject_id, @reviewer_revision, @request_key,
    @import_revision, @evidence_sha256, @checklist_version, @auth_generation, @recovery_id
) ON CONFLICT DO NOTHING RETURNING *;

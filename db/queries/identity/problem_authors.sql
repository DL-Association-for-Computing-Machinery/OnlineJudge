-- name: ProblemAuthorForShare :one
SELECT subject_id, enabled, revision FROM identity_problem_authors WHERE subject_id = @subject_id FOR SHARE;

-- name: ProblemAuthorForUpdate :one
SELECT subject_id, enabled, revision FROM identity_problem_authors WHERE subject_id = @subject_id FOR UPDATE;

-- name: ProblemAuthor :one
SELECT subject_id, enabled, revision FROM identity_problem_authors WHERE subject_id = @subject_id;

-- name: AccountForRoleChange :one
SELECT id, enabled FROM identity_credentials WHERE id = @id FOR UPDATE;

-- name: CreateProblemAuthor :one
INSERT INTO identity_problem_authors(subject_id,enabled,revision) VALUES(@subject_id,@enabled,1)
RETURNING subject_id,enabled,revision;

-- name: ChangeProblemAuthor :one
UPDATE identity_problem_authors SET enabled = @enabled,revision = revision+1
WHERE subject_id = @subject_id AND revision = @expected_revision AND revision < 9223372036854775807
RETURNING subject_id,enabled,revision;

-- name: AuditProblemAuthor :exec
INSERT INTO identity_problem_author_audit(subject_id,previous_revision,revision,enabled,maintenance_operator,reason)
VALUES(@subject_id,@previous_revision,@revision,@enabled,@maintenance_operator,@reason);

-- name: PublicationByPublisherKey :one
SELECT * FROM problem_publications WHERE publisher_id = @publisher_id AND request_key = @request_key;

-- name: LatestProblemPublication :one
SELECT * FROM problem_publications WHERE problem_id = @problem_id ORDER BY revision DESC LIMIT 1;

-- name: InsertProblemPublication :one
INSERT INTO problem_publications (problem_id, revision, version, title, statement_sha256, statement_size,
    import_revision, formal_plan_sha256, execution_version, execution_sha256, review_id, assignment_revision, publisher_id, publisher_revision, request_key)
VALUES (@problem_id, @revision, @version, @title, @statement_sha256, @statement_size,
    @import_revision, @formal_plan_sha256, @execution_version, @execution_sha256, @review_id, @assignment_revision, @publisher_id, @publisher_revision, @request_key)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: CatalogProblem :one
SELECT p.problem_id, p.revision, p.version, p.title, p.statement_sha256, p.statement_size,
       r.source_id, r.content_version, r.import_revision, r.source_lock_sha256, r.formal_plan_sha256
FROM problem_publications p JOIN problem_review_submissions r ON r.id = p.review_id
JOIN problem_review_approvals a ON a.review_id=p.review_id AND a.assignment_revision=p.assignment_revision
WHERE p.problem_id = @problem_id
  AND (a.policy_version = 'oj.review-approval.aplusb-local.v1') = @local_only::boolean
  AND p.import_revision = r.import_revision AND p.formal_plan_sha256 = r.formal_plan_sha256
  AND p.revision = (SELECT max(h.revision) FROM problem_publications h WHERE h.problem_id = p.problem_id);

-- name: PublishedVersionOwner :one
SELECT r.owner_subject_id FROM problem_publications p
JOIN problem_review_submissions r ON r.id = p.review_id
WHERE p.problem_id = @problem_id AND p.revision = @revision;

-- name: CatalogProblems :many
SELECT p.problem_id, p.version, p.title
FROM problem_publications p JOIN problem_review_submissions r ON r.id = p.review_id
JOIN problem_review_approvals a ON a.review_id=p.review_id AND a.assignment_revision=p.assignment_revision
WHERE p.problem_id > @after_id
  AND (a.policy_version = 'oj.review-approval.aplusb-local.v1') = @local_only::boolean
  AND p.revision = (SELECT max(h.revision) FROM problem_publications h WHERE h.problem_id = p.problem_id)
  AND p.import_revision = r.import_revision AND p.formal_plan_sha256 = r.formal_plan_sha256
  AND strpos(lower(p.title), lower(@title_query::text)) > 0
ORDER BY p.problem_id LIMIT 21;

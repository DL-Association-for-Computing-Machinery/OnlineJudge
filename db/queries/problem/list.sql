-- name: ListOwnDraftsFirst :many
SELECT id, request_key, source_id, content_version, request_sha256, state, validation_state, created_at
FROM problem_drafts
WHERE owner_subject_id = sqlc.arg(owner)
ORDER BY created_at DESC, id COLLATE "C" DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: ListOwnDraftsAfter :many
SELECT id, request_key, source_id, content_version, request_sha256, state, validation_state, created_at
FROM problem_drafts
WHERE owner_subject_id = sqlc.arg(owner)
  AND (created_at, id COLLATE "C") < (sqlc.arg(created_at)::timestamptz, sqlc.arg(id)::text COLLATE "C")
ORDER BY created_at DESC, id COLLATE "C" DESC
LIMIT sqlc.arg(row_limit)::integer;

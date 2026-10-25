-- name: CreateSubmission :one
INSERT INTO submissions (id, subject_id, context_id, idempotency_key, request_sha256,
    material_version, manifest_sha256, language, source_code)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (subject_id, context_id, idempotency_key) DO NOTHING
RETURNING id;

-- name: CreateTask :exec
INSERT INTO judge_tasks (submission_id) VALUES ($1);

-- name: GetReceipt :one
SELECT s.id, s.request_sha256, s.material_version, s.language, s.created_at,
    t.state, t.verdict
FROM submissions s JOIN judge_tasks t ON t.submission_id = s.id
WHERE s.subject_id = $1 AND s.context_id = $2 AND s.idempotency_key = $3;

-- name: GetSubmission :one
SELECT s.id, s.request_sha256, s.material_version, s.language, s.created_at,
    t.state, t.verdict
FROM submissions s JOIN judge_tasks t ON t.submission_id = s.id
WHERE s.id = $1 AND s.subject_id = $2 AND s.context_id = $3;

-- name: RegisterMaterial :exec
INSERT INTO submission_materials (version_id, manifest_sha256, manifest)
VALUES ($1, $2, $3) ON CONFLICT (version_id) DO NOTHING;

-- name: GetMaterial :one
SELECT manifest_sha256, manifest FROM submission_materials WHERE version_id = $1;

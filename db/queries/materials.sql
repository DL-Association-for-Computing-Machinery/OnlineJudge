-- name: GetLeaseMaterials :one
-- Caller holds the node and current task/attempt locks. Evaluate expiry after
-- those locks have been acquired, not before a potentially long lock wait.
SELECT s.id, s.material_version, s.manifest_sha256, s.language, s.source_code, m.manifest
FROM judge_attempts a JOIN submissions s ON s.id=a.submission_id
JOIN submission_materials m ON m.version_id=s.material_version
WHERE a.token=$1 AND a.state='leased' AND a.lease_until>clock_timestamp();

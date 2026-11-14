-- name: LatestPlatformAdmin :one
SELECT subject_id, revision, enabled
FROM identity_platform_admin_history
WHERE subject_id = @subject_id
ORDER BY revision DESC LIMIT 1;

-- name: AppendPlatformAdmin :exec
INSERT INTO identity_platform_admin_history(subject_id, revision, enabled, maintenance_operator, reason)
VALUES (@subject_id, @revision, @enabled, @maintenance_operator, @reason);

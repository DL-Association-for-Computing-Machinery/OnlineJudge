-- name: RecoveryState :one
SELECT generation, enabled FROM identity_session_recovery WHERE singleton;

-- name: RecoveryStateForShare :one
SELECT generation, enabled FROM identity_session_recovery WHERE singleton FOR SHARE;

-- name: BeginRecovery :one
UPDATE identity_session_recovery SET generation = @generation, enabled = false
WHERE singleton RETURNING generation, enabled;

-- name: FinishRecovery :execrows
UPDATE identity_session_recovery SET enabled = true
WHERE singleton AND generation = @expected_generation AND NOT enabled;

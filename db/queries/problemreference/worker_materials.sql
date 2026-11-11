-- name: ReferenceWorkerClaimForShare :one
SELECT * FROM problem_reference_worker_claim WHERE singleton FOR SHARE;

-- name: ReferenceWorkerClaimRemaining :one
-- Run AFTER the reservation row lock is acquired: lock waits consume the lease.
SELECT floor(extract(epoch FROM (lease_until - clock_timestamp())) * 1000)::bigint AS remaining_ms
FROM problem_reference_worker_claim WHERE singleton;

-- name: ReferenceWorkerClaimForUpdate :one
SELECT * FROM problem_reference_worker_claim WHERE singleton FOR UPDATE;

-- name: RenewReferenceWorkerClaim :one
-- Acquire the singleton row lock first. The database clock is read after any
-- wait, and neither a replaced token nor an expired token can be renewed.
UPDATE problem_reference_worker_claim
SET lease_until = clock_timestamp() + interval '30 seconds'
WHERE singleton AND request_id = @request_id AND node_id = @node_id
    AND credential_version = @credential_version AND lease_token = @lease_token
    AND plan_sha256 = @plan_sha256 AND lease_until > clock_timestamp()
RETURNING lease_until;

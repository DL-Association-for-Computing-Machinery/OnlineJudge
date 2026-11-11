-- name: ReferenceWorkerClaimForShare :one
SELECT * FROM problem_reference_worker_claim WHERE singleton FOR SHARE;

-- name: ReferenceWorkerClaimRemaining :one
-- Run AFTER the reservation row lock is acquired: lock waits consume the lease.
SELECT floor(extract(epoch FROM (lease_until - clock_timestamp())) * 1000)::bigint AS remaining_ms
FROM problem_reference_worker_claim WHERE singleton;

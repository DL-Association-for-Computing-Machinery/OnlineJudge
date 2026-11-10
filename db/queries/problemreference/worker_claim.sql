-- name: ReferenceRequestByID :one
SELECT * FROM problem_reference_requests WHERE id = @id;

-- name: ReferenceRequestByIDForUpdate :one
SELECT * FROM problem_reference_requests WHERE id = @id FOR UPDATE;

-- name: ReferenceExpectedPlanForShare :one
SELECT plan_encoding, plan_sha256 FROM problem_reference_expected_plans
WHERE request_id = @request_id FOR SHARE;

-- name: ReserveReferenceWorkerClaim :one
INSERT INTO problem_reference_worker_claim (singleton, request_id, node_id, credential_version, lease_token, plan_sha256, lease_until)
VALUES (true, @request_id, @node_id, @credential_version, @lease_token, @plan_sha256, clock_timestamp() + interval '30 seconds')
ON CONFLICT (singleton) DO UPDATE SET
    request_id = excluded.request_id, node_id = excluded.node_id,
    credential_version = excluded.credential_version, lease_token = excluded.lease_token,
    plan_sha256 = excluded.plan_sha256, lease_until = clock_timestamp() + interval '30 seconds'
WHERE problem_reference_worker_claim.lease_until <= clock_timestamp()
RETURNING lease_until;

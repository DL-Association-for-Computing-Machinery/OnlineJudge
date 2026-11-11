-- name: ReferenceWorkerReport :one
SELECT * FROM problem_reference_worker_reports WHERE request_id = @request_id;

-- name: InsertReferenceWorkerReport :exec
INSERT INTO problem_reference_worker_reports(request_id,node_id,credential_version,token_sha256,plan_sha256,evidence,evidence_sha256)
VALUES (@request_id,@node_id,@credential_version,@token_sha256,@plan_sha256,@evidence,encode(sha256(@evidence),'hex'));

-- name: MarkReferenceReported :execrows
UPDATE problem_reference_requests SET state = 'reported' WHERE id = @id AND state = 'queued';

-- name: ConsumeLiveReferenceClaim :one
-- Evaluate expiry after locking the singleton row and immediately before the
-- terminal state change. A replay never calls this statement.
UPDATE problem_reference_worker_claim SET lease_until = clock_timestamp()
WHERE singleton AND request_id = @request_id AND node_id = @node_id
    AND credential_version = @credential_version AND lease_token = @lease_token
    AND plan_sha256 = @plan_sha256 AND lease_until > clock_timestamp()
RETURNING lease_until;

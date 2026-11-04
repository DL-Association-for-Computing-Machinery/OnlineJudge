-- name: ReferenceRequest :one
SELECT * FROM problem_reference_requests WHERE owner_subject_id = @owner AND request_key = @request_key;

-- name: ReferenceRequestForUpdate :one
SELECT * FROM problem_reference_requests WHERE owner_subject_id = @owner AND request_key = @request_key FOR UPDATE;

-- name: CreateReferenceRequest :one
INSERT INTO problem_reference_requests(id,owner_subject_id,request_key,draft_id,source_id,content_version,import_revision,source_lock_sha256,identity_protocol,profile_encoding,profile_sha256,auth_generation,recovery_id,author_revision)
VALUES(@id,@owner,@request_key,@draft_id,@source_id,@content_version,@import_revision,@source_lock_sha256,@identity_protocol,@profile_encoding,@profile_sha256,@auth_generation,@recovery_id,@author_revision)
ON CONFLICT(owner_subject_id,request_key) DO NOTHING
RETURNING *;

-- name: CancelReferenceRequest :one
UPDATE problem_reference_requests SET state='cancelled',cancelled_at=clock_timestamp()
WHERE owner_subject_id = @owner AND request_key = @request_key AND state='queued' RETURNING *;

-- name: LockReferencePolicy :one
SELECT global_limit, owner_limit FROM problem_reference_request_policy WHERE singleton FOR UPDATE;

-- name: SetReferencePolicy :exec
INSERT INTO problem_reference_request_policy(singleton,global_limit,owner_limit) VALUES(true,@global_limit,@owner_limit)
ON CONFLICT(singleton) DO UPDATE SET global_limit=excluded.global_limit,owner_limit=excluded.owner_limit;

-- name: ReferenceQueuedCounts :one
-- At the global window limit admission already fails; below it both counts
-- are exact. This bounds work even after lowering the configured global cap.
WITH pending AS MATERIALIZED (
    SELECT owner_subject_id FROM problem_reference_requests WHERE state='queued'
    LIMIT sqlc.arg(maximum)::bigint
)
SELECT count(*)::bigint AS total,
    count(*) FILTER (WHERE owner_subject_id = sqlc.arg(owner)::text)::bigint AS personal
FROM pending;

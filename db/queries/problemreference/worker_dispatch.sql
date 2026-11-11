-- name: ReferenceDispatchCandidates :many
-- Limit the source queue scan before checking Plan and history for each row.
WITH candidates AS MATERIALIZED (
    SELECT id FROM problem_reference_requests
    WHERE state = 'queued' AND source_id <> '' AND source_id = @source_id AND id COLLATE "C" > sqlc.arg(after_id)::text
    ORDER BY id COLLATE "C" LIMIT 8
)
SELECT c.id, coalesce(p.plan_sha256, '')::text AS plan_sha256,
       (h.request_id IS NOT NULL)::boolean AS dispatched
FROM candidates c
LEFT JOIN LATERAL (
    SELECT plan_sha256 FROM problem_reference_expected_plans WHERE request_id = c.id LIMIT 1
) p ON true
LEFT JOIN LATERAL (
    SELECT request_id FROM problem_reference_dispatch_history WHERE request_id = c.id LIMIT 1
) h ON true
ORDER BY c.id COLLATE "C";

-- name: HasReferenceDispatch :one
SELECT EXISTS (SELECT 1 FROM problem_reference_dispatch_history WHERE request_id = @request_id);

-- name: RecordReferenceDispatch :execrows
INSERT INTO problem_reference_dispatch_history(request_id, first_claim_at)
VALUES (@request_id, clock_timestamp()) ON CONFLICT (request_id) DO NOTHING;

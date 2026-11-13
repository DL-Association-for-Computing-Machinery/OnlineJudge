-- name: DeleteEmailPayload :exec
DELETE FROM identity_email_payloads WHERE token_id = @payload_id;

-- name: SweepExpiredEmailPayloads :one
WITH batch AS MATERIALIZED (
    SELECT token_id
    FROM identity_email_payloads
    WHERE token_id > sqlc.arg(after_id)::text
    ORDER BY token_id
    LIMIT sqlc.arg(batch_size)::integer
), expired AS MATERIALIZED (
    SELECT locked.token_id
    FROM (SELECT token_id FROM batch ORDER BY token_id) candidate
    CROSS JOIN LATERAL (
        SELECT p.token_id
        FROM identity_email_payloads p
        WHERE p.token_id = candidate.token_id AND p.send_until <= statement_timestamp()
        LIMIT 1 FOR UPDATE SKIP LOCKED
    ) locked
), removed AS (
    DELETE FROM identity_email_payloads p
    WHERE p.token_id = ANY(ARRAY(SELECT token_id FROM expired))
    RETURNING p.token_id
)
SELECT COALESCE((SELECT token_id FROM batch ORDER BY token_id DESC LIMIT 1), '')::text AS last_id,
       (SELECT count(*) FROM batch) AS visited,
       (SELECT count(*) FROM removed) AS removed;

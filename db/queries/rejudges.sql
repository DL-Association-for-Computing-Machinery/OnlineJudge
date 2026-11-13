-- name: RejudgeSession :one
SELECT current_database()::text AS database_name, session_user::text AS operator, EXISTS (
    SELECT 1 FROM pg_catalog.pg_auth_members m
    JOIN pg_catalog.pg_roles r ON r.oid = m.roleid
    JOIN pg_catalog.pg_roles u ON u.oid = m.member
    WHERE r.rolname = 'oj_rejudge_operator' AND NOT r.rolcanlogin
      AND u.rolname = session_user AND u.rolcanlogin
) AS allowed;

-- name: ReadRejudge :one
SELECT * FROM submission_rejudges WHERE operator=$1 AND request_key=$2;

-- name: LockRejudgeTarget :one
SELECT s.subject_id, s.context_id, s.material_version, s.manifest_sha256, t.judge_generation, t.state,
    EXISTS (
        SELECT 1 FROM judge_attempts a WHERE a.submission_id=t.submission_id
        AND a.judge_generation=t.judge_generation AND a.recovery_generation=t.recovery_generation
        AND a.attempt_number=t.attempt_number AND a.state='completed' AND a.result_payload->>'Verdict'=t.verdict
    ) AS completed
FROM judge_tasks t JOIN submissions s ON s.id=t.submission_id
WHERE t.submission_id=$1 FOR UPDATE OF t;

-- name: EnqueueRejudge :execrows
UPDATE judge_tasks SET state='queued', verdict=NULL, attempt_number=0, judge_generation=judge_generation+1,
    queue_version=sqlc.arg(version_id)::text, virtual_finish=sqlc.arg(rank)::bigint, enqueue_order=nextval('training_enqueue_order')
WHERE submission_id=sqlc.arg(submission_id) AND judge_generation=sqlc.arg(previous_generation) AND state='finished';

-- name: InsertRejudge :one
INSERT INTO submission_rejudges (operator, request_key, submission_id, previous_generation, material_version, manifest_sha256, reason)
VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *;

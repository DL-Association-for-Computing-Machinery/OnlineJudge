-- name: LockNode :one
SELECT * FROM worker_nodes WHERE id=$1 FOR UPDATE;

-- name: CountNodeLeases :one
SELECT count(*) FROM judge_attempts WHERE node_id=$1 AND state='leased' AND lease_until>clock_timestamp();

-- name: ClaimCandidate :one
SELECT t.submission_id FROM judge_tasks t JOIN submissions s ON s.id=t.submission_id
WHERE t.state='queued' AND s.material_version=$1
ORDER BY s.created_at, s.id LIMIT 1 FOR UPDATE OF t SKIP LOCKED;

-- name: BeginAttempt :one
UPDATE judge_tasks SET state='running', attempt_number=attempt_number+1
WHERE submission_id=$1 AND state='queued'
RETURNING *;

-- name: InsertAttempt :one
INSERT INTO judge_attempts(token, submission_id, judge_generation, recovery_generation,
    attempt_number, node_id, credential_version, lease_until)
VALUES ($1,$2,$3,$4,$5,$6,$7,clock_timestamp()+sqlc.arg(lease_seconds)::integer*interval '1 second')
RETURNING lease_until;

-- name: LockAttempt :one
SELECT a.*, t.state AS task_state, t.attempt_number AS current_attempt FROM judge_attempts a JOIN judge_tasks t USING(submission_id)
WHERE a.token=$1 AND a.node_id=$2 AND a.credential_version=$3
AND a.judge_generation=t.judge_generation AND a.recovery_generation=t.recovery_generation
FOR UPDATE OF t,a;

-- name: RenewAttempt :one
UPDATE judge_attempts SET lease_until=clock_timestamp()+sqlc.arg(lease_seconds)::integer*interval '1 second'
WHERE token=$1 AND state='leased' AND lease_until>clock_timestamp()
RETURNING lease_until;

-- name: CompleteAttempt :execrows
UPDATE judge_attempts SET state='completed',result_sha256=$2,result_payload=$3
WHERE token=$1 AND state='leased' AND lease_until>clock_timestamp();

-- name: FinishTask :exec
UPDATE judge_tasks SET state='finished',verdict=$2 WHERE submission_id=$1;

-- name: ExpiredAttempt :one
SELECT a.token, a.submission_id, a.attempt_number FROM judge_attempts a
JOIN judge_tasks t USING(submission_id) JOIN worker_nodes n ON n.id=a.node_id
WHERE a.state='leased' AND t.state='running'
AND a.judge_generation=t.judge_generation AND a.recovery_generation=t.recovery_generation
AND a.attempt_number=t.attempt_number
AND (a.lease_until<=clock_timestamp() OR NOT n.enabled OR a.credential_version<>n.credential_version)
ORDER BY a.lease_until LIMIT 1 FOR UPDATE OF t,a SKIP LOCKED;

-- name: ExpireAttempt :exec
UPDATE judge_attempts SET state='expired' WHERE token=$1;

-- name: RecoverTask :exec
UPDATE judge_tasks SET state=CASE WHEN attempt_number>=3 THEN 'failed' ELSE 'queued' END
WHERE submission_id=$1;

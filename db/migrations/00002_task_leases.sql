-- +goose Up
CREATE TABLE worker_nodes (
    id text PRIMARY KEY,
    credential_version bigint NOT NULL CHECK (credential_version > 0),
    enabled boolean NOT NULL DEFAULT true,
    capacity integer NOT NULL DEFAULT 1 CHECK (capacity BETWEEN 1 AND 8)
);

CREATE TABLE judge_attempts (
    token text PRIMARY KEY,
    submission_id text NOT NULL REFERENCES judge_tasks(submission_id),
    judge_generation bigint NOT NULL,
    recovery_generation bigint NOT NULL,
    attempt_number integer NOT NULL,
    node_id text NOT NULL REFERENCES worker_nodes(id),
    credential_version bigint NOT NULL,
    lease_until timestamptz NOT NULL,
    state text NOT NULL DEFAULT 'leased' CHECK (state IN ('leased','completed','expired')),
    result_sha256 text,
    result_payload jsonb,
    UNIQUE (submission_id, judge_generation, recovery_generation, attempt_number),
    CHECK ((state='completed' AND result_sha256 IS NOT NULL AND result_sha256 ~ '^[a-f0-9]{64}$'
            AND result_payload IS NOT NULL AND jsonb_typeof(result_payload)='object') OR
           (state<>'completed' AND result_sha256 IS NULL AND result_payload IS NULL))
);

CREATE INDEX judge_attempts_active_node ON judge_attempts(node_id, lease_until) WHERE state='leased';
CREATE INDEX judge_attempts_expired ON judge_attempts(lease_until) WHERE state='leased';
CREATE INDEX judge_tasks_queued ON judge_tasks(submission_id) WHERE state='queued';

-- +goose Down
-- Serialize the guard with acceptance, not just other migration processes.
LOCK TABLE submissions IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM submissions) THEN
        RAISE EXCEPTION 'cannot roll back accepted data; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP INDEX judge_tasks_queued;
DROP TABLE judge_attempts;
DROP TABLE worker_nodes;

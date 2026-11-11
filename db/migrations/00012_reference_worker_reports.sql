-- +goose Up
ALTER TABLE problem_reference_requests
    DROP CONSTRAINT problem_reference_requests_state_check,
    DROP CONSTRAINT problem_reference_requests_check,
    ADD CONSTRAINT problem_reference_requests_state_check CHECK (state IN ('queued', 'cancelled', 'reported')),
    ADD CONSTRAINT problem_reference_requests_check CHECK ((state = 'cancelled') = (cancelled_at IS NOT NULL));

CREATE TABLE problem_reference_worker_reports (
    request_id text PRIMARY KEY REFERENCES problem_reference_requests(id),
    node_id text NOT NULL CHECK (node_id ~ '^[A-Za-z0-9_-]{1,128}$'),
    credential_version bigint NOT NULL CHECK (credential_version > 0),
    token_sha256 text NOT NULL CHECK (token_sha256 ~ '^[a-f0-9]{64}$'),
    plan_sha256 text NOT NULL CHECK (plan_sha256 ~ '^[a-f0-9]{64}$'),
    evidence bytea NOT NULL CHECK (octet_length(evidence) BETWEEN 1 AND 16384),
    evidence_sha256 text NOT NULL CHECK (evidence_sha256 = encode(sha256(evidence), 'hex')),
    accepted_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(accepted_at))
);

-- +goose Down
-- Both first acceptance and exact replay access requests before reports.
-- Dropping the report foreign key also needs the request table lock.
LOCK TABLE problem_reference_requests, problem_reference_worker_reports IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM problem_reference_worker_reports)
       OR EXISTS (SELECT 1 FROM problem_reference_requests WHERE state = 'reported') THEN
        RAISE EXCEPTION 'cannot discard accepted reference report state; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE problem_reference_worker_reports;
ALTER TABLE problem_reference_requests
    DROP CONSTRAINT problem_reference_requests_state_check,
    DROP CONSTRAINT problem_reference_requests_check,
    ADD CONSTRAINT problem_reference_requests_state_check CHECK (state IN ('queued', 'cancelled')),
    ADD CONSTRAINT problem_reference_requests_check CHECK ((state = 'queued' AND cancelled_at IS NULL) OR (state = 'cancelled' AND cancelled_at IS NOT NULL));

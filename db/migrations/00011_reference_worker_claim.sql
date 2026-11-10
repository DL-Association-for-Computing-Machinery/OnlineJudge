-- +goose Up
-- One reference reservation at a time. The expired row is reused, keeping
-- storage bounded. Neither this lease nor its expiry changes request state.
CREATE TABLE problem_reference_worker_claim (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    request_id text NOT NULL REFERENCES problem_reference_expected_plans(request_id),
    node_id text NOT NULL REFERENCES worker_nodes(id),
    credential_version bigint NOT NULL CHECK (credential_version > 0),
    lease_token text NOT NULL CHECK (lease_token ~ '^[a-f0-9]{64}$'),
    plan_sha256 text NOT NULL CHECK (plan_sha256 ~ '^[a-f0-9]{64}$'),
    lease_until timestamptz NOT NULL CHECK (isfinite(lease_until))
);

-- +goose Down
-- Acquire the FK parents in claim order before removing their triggers.
LOCK TABLE worker_nodes, problem_reference_expected_plans, problem_reference_worker_claim IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM problem_reference_worker_claim) THEN
        RAISE EXCEPTION 'cannot discard reference claim state; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE problem_reference_worker_claim;

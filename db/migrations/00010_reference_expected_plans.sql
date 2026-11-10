-- +goose Up
CREATE TABLE problem_reference_expected_plans (
    request_id text PRIMARY KEY REFERENCES problem_reference_requests(id),
    plan_encoding bytea NOT NULL CHECK (octet_length(plan_encoding) BETWEEN 1 AND 65536),
    plan_sha256 text NOT NULL CHECK (plan_sha256 = encode(sha256(plan_encoding), 'hex'))
);

-- +goose Down
-- Lock the parent first, in the same order as plan writers and FK teardown.
LOCK TABLE problem_reference_requests, problem_reference_expected_plans IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM problem_reference_expected_plans) THEN
        RAISE EXCEPTION 'cannot discard expected reference plans; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE problem_reference_expected_plans;

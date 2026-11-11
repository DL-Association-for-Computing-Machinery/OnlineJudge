-- +goose Up
ALTER TABLE problem_reference_expected_plans
    ADD COLUMN differential_encoding bytea,
    ADD COLUMN differential_sha256 text,
    ADD CONSTRAINT problem_reference_expected_plans_differential_pair CHECK (
        (differential_encoding IS NULL AND differential_sha256 IS NULL) OR
        (differential_encoding IS NOT NULL AND differential_sha256 IS NOT NULL AND
         octet_length(differential_encoding) BETWEEN 1 AND 2048 AND
         differential_sha256 = encode(sha256(differential_encoding), 'hex'))
    );

-- +goose Down
-- A writer locks the request before its expected-plan row.
LOCK TABLE problem_reference_requests, problem_reference_expected_plans IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM problem_reference_expected_plans WHERE differential_encoding IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot discard differential reference plans; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE problem_reference_expected_plans
    DROP CONSTRAINT problem_reference_expected_plans_differential_pair,
    DROP COLUMN differential_encoding,
    DROP COLUMN differential_sha256;

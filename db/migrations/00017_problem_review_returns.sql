-- +goose Up
CREATE TABLE problem_review_returns (
    review_id text PRIMARY KEY REFERENCES problem_review_submissions(id),
    material_sha256 text NOT NULL UNIQUE CHECK (material_sha256 ~ '^[a-f0-9]{64}$'),
    reviewer_subject_id text NOT NULL REFERENCES identity_credentials(id),
    request_key text COLLATE "C" NOT NULL CHECK (request_key ~ '^[A-Za-z0-9_-]{16,128}$'),
    reason text NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 512),
    auth_generation bigint NOT NULL CHECK (auth_generation > 0),
    recovery_id text NOT NULL CHECK (recovery_id ~ '^[a-f0-9]{32}$'),
    reviewer_revision bigint NOT NULL CHECK (reviewer_revision > 0),
    assignment_revision bigint NOT NULL CHECK (assignment_revision > 0),
    returned_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(returned_at)),
    UNIQUE (reviewer_subject_id, request_key)
);

-- +goose Down
-- Drain readers and writers. Account locks precede both review tables.
SET LOCAL lock_timeout = '5s';
LOCK TABLE identity_credentials, problem_review_submissions,
    problem_review_returns IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM problem_review_returns) THEN
        RAISE EXCEPTION 'cannot discard review returns; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE problem_review_returns;

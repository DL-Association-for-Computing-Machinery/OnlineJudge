-- +goose Up
CREATE TABLE problem_solution_confirmations (
    draft_id text NOT NULL CHECK (draft_id ~ '^[a-f0-9]{32}$'),
    import_revision text NOT NULL CHECK (import_revision ~ '^[a-f0-9]{64}$'),
    formal_plan_sha256 text NOT NULL CHECK (formal_plan_sha256 ~ '^[a-f0-9]{64}$'),
    review_id text NOT NULL REFERENCES problem_review_submissions(id),
    owner_subject_id text NOT NULL CHECK (owner_subject_id ~ '^[a-f0-9]{32}$'),
    author_revision bigint NOT NULL CHECK (author_revision > 0),
    auth_generation bigint NOT NULL CHECK (auth_generation > 0),
    recovery_id text NOT NULL CHECK (recovery_id ~ '^[a-f0-9]{32}$'),
    confirmed_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(confirmed_at)),
    PRIMARY KEY (draft_id, import_revision, formal_plan_sha256)
);
CREATE INDEX problem_solution_confirmations_review ON problem_solution_confirmations(review_id);

-- +goose Down
SET LOCAL lock_timeout = '5s';
LOCK TABLE identity_credentials, problem_review_submissions, problem_solution_confirmations IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM problem_solution_confirmations) THEN
        RAISE EXCEPTION 'cannot discard solution confirmations; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE problem_solution_confirmations;

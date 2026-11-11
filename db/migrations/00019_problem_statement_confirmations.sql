-- +goose Up
CREATE TABLE problem_statement_confirmations (
    review_id text NOT NULL,
    assignment_revision bigint NOT NULL CHECK (assignment_revision > 0),
    reviewer_subject_id text NOT NULL REFERENCES identity_credentials(id),
    reviewer_revision bigint NOT NULL CHECK (reviewer_revision > 0),
    request_key text COLLATE "C" NOT NULL CHECK (request_key ~ '^[A-Za-z0-9_-]{16,128}$'),
    import_revision text NOT NULL CHECK (import_revision ~ '^[a-f0-9]{64}$'),
    evidence_sha256 text NOT NULL CHECK (evidence_sha256 ~ '^[a-f0-9]{64}$'),
    checklist_version text NOT NULL CHECK (checklist_version = 'oj.statement-semantics.v1'),
    auth_generation bigint NOT NULL CHECK (auth_generation > 0),
    recovery_id text NOT NULL CHECK (recovery_id ~ '^[a-f0-9]{32}$'),
    confirmed_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(confirmed_at)),
    PRIMARY KEY (review_id, assignment_revision),
    FOREIGN KEY (review_id, assignment_revision) REFERENCES problem_review_assignment_history(review_id, revision),
    CONSTRAINT problem_statement_confirmations_reviewer_key UNIQUE (reviewer_subject_id, request_key)
);

-- +goose Down
-- Stop the matching binaries before rolling back; preserve any recorded audit.
SET LOCAL lock_timeout = '5s';
LOCK TABLE identity_credentials, problem_review_submissions,
    problem_review_assignment_history, problem_statement_confirmations IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM problem_statement_confirmations) THEN
        RAISE EXCEPTION 'cannot discard statement confirmations; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE problem_statement_confirmations;

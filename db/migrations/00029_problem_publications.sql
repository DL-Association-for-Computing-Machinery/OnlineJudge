-- +goose Up
SET LOCAL lock_timeout = '5s';
-- A registered judge manifest alone is not a production admission. This
-- ledger starts empty; only the future evidence admission path may fill it.
CREATE TABLE problem_execution_admissions (
    import_revision text NOT NULL CHECK (import_revision ~ '^[a-f0-9]{64}$'),
    formal_plan_sha256 text NOT NULL CHECK (formal_plan_sha256 ~ '^[a-f0-9]{64}$'),
    execution_version text NOT NULL,
    execution_sha256 text NOT NULL,
    PRIMARY KEY (import_revision, formal_plan_sha256, execution_version, execution_sha256),
    FOREIGN KEY (execution_version, execution_sha256) REFERENCES submission_materials (version_id, manifest_sha256)
);

CREATE TABLE problem_publications (
    problem_id text COLLATE "C" NOT NULL CHECK (problem_id ~ '^[a-f0-9]{32}$'),
    revision bigint NOT NULL CHECK (revision > 0),
    version text NOT NULL CHECK (version ~ '^[a-f0-9]{64}$'),
    title text NOT NULL CHECK (octet_length(title) BETWEEN 1 AND 512),
    statement_sha256 text NOT NULL CHECK (statement_sha256 ~ '^[a-f0-9]{64}$'),
    statement_size bigint NOT NULL CHECK (statement_size BETWEEN 1 AND 7340032),
    import_revision text NOT NULL,
    formal_plan_sha256 text NOT NULL,
    execution_version text NOT NULL,
    execution_sha256 text NOT NULL,
    review_id text NOT NULL,
    assignment_revision bigint NOT NULL,
    publisher_id text NOT NULL,
    publisher_revision bigint NOT NULL,
    request_key text COLLATE "C" NOT NULL CHECK (request_key ~ '^[A-Za-z0-9_-]{16,128}$'),
    published_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(published_at)),
    PRIMARY KEY (problem_id, revision),
    UNIQUE (problem_id, version),
    UNIQUE (publisher_id, request_key),
    UNIQUE (review_id, assignment_revision),
    FOREIGN KEY (review_id, assignment_revision) REFERENCES problem_review_approvals (review_id, assignment_revision),
    FOREIGN KEY (publisher_id, publisher_revision) REFERENCES identity_problem_publisher_history (subject_id, revision),
    FOREIGN KEY (import_revision, formal_plan_sha256, execution_version, execution_sha256)
        REFERENCES problem_execution_admissions (import_revision, formal_plan_sha256, execution_version, execution_sha256)
);

-- +goose Down
SET LOCAL lock_timeout = '5s';
LOCK TABLE identity_problem_publisher_history, problem_review_approvals, submission_materials,
    problem_execution_admissions, problem_publications IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM problem_publications) OR EXISTS (SELECT 1 FROM problem_execution_admissions) THEN
        RAISE EXCEPTION 'cannot discard publication history; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE problem_publications;
DROP TABLE problem_execution_admissions;

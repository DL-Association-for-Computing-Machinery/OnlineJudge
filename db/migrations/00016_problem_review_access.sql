-- +goose Up
CREATE TABLE identity_problem_reviewer_history (
    subject_id text NOT NULL REFERENCES identity_credentials(id),
    revision bigint NOT NULL CHECK (revision > 0),
    enabled boolean NOT NULL,
    maintenance_operator text NOT NULL CHECK (octet_length(maintenance_operator) BETWEEN 1 AND 128),
    reason text NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 512),
    changed_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(changed_at)),
    PRIMARY KEY (subject_id, revision)
);

CREATE TABLE problem_review_assignment_history (
    review_id text NOT NULL REFERENCES problem_review_submissions(id),
    revision bigint NOT NULL CHECK (revision > 0),
    reviewer_subject_id text,
    reviewer_revision bigint,
    maintenance_operator text NOT NULL CHECK (octet_length(maintenance_operator) BETWEEN 1 AND 128),
    reason text NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 512),
    changed_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(changed_at)),
    PRIMARY KEY (review_id, revision),
    FOREIGN KEY (reviewer_subject_id, reviewer_revision)
        REFERENCES identity_problem_reviewer_history(subject_id, revision) MATCH FULL
);
CREATE INDEX problem_review_assignment_reviewer
    ON problem_review_assignment_history(reviewer_subject_id, reviewer_revision)
    WHERE reviewer_subject_id IS NOT NULL;

-- +goose Down
-- Drain old readers and writers. Credentials first drains all operations
-- that lock an account before reading the role and item. Withdrawal takes
-- only the item before INSERT, whose FK opens the role table even for NULLs.
SET LOCAL lock_timeout = '5s';
LOCK TABLE identity_credentials, problem_review_submissions,
    identity_problem_reviewer_history, problem_review_assignment_history IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM identity_problem_reviewer_history)
        OR EXISTS (SELECT 1 FROM problem_review_assignment_history) THEN
        RAISE EXCEPTION 'cannot discard review access history; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE problem_review_assignment_history;
DROP TABLE identity_problem_reviewer_history;

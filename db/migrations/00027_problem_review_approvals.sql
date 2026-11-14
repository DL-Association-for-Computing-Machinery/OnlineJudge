-- +goose Up
SET LOCAL lock_timeout = '5s';
ALTER TABLE problem_review_assignment_history ADD CONSTRAINT problem_review_assignment_approval_binding
    UNIQUE (review_id, revision, reviewer_subject_id, reviewer_revision);
CREATE TABLE problem_review_approvals (
    review_id text NOT NULL,
    assignment_revision bigint NOT NULL CHECK (assignment_revision > 0),
    reviewer_subject_id text NOT NULL,
    reviewer_revision bigint NOT NULL CHECK (reviewer_revision > 0),
    request_key text COLLATE "C" NOT NULL CHECK (request_key ~ '^[A-Za-z0-9_-]{16,128}$'),
    material_sha256 text NOT NULL CHECK (material_sha256 ~ '^[a-f0-9]{64}$'),
    snapshot_sha256 text NOT NULL CHECK (snapshot_sha256 ~ '^[a-f0-9]{64}$'),
    policy_version text NOT NULL CHECK (octet_length(policy_version) BETWEEN 1 AND 80),
    reason text NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 512),
    auth_generation bigint NOT NULL CHECK (auth_generation > 0),
    recovery_id text NOT NULL CHECK (recovery_id ~ '^[a-f0-9]{32}$'),
    approved_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(approved_at)),
    PRIMARY KEY (review_id, assignment_revision),
    FOREIGN KEY (review_id, assignment_revision, reviewer_subject_id, reviewer_revision)
        REFERENCES problem_review_assignment_history(review_id, revision, reviewer_subject_id, reviewer_revision),
    UNIQUE (reviewer_subject_id, request_key)
);
CREATE INDEX problem_review_approval_material ON problem_review_approvals (material_sha256);

-- +goose Down
SET LOCAL lock_timeout = '5s';
LOCK TABLE problem_review_assignment_history, problem_review_approvals IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM problem_review_approvals) THEN
        RAISE EXCEPTION 'cannot discard review approvals; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE problem_review_approvals;
ALTER TABLE problem_review_assignment_history DROP CONSTRAINT problem_review_assignment_approval_binding;

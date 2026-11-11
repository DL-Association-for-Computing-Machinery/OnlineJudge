-- +goose Up
CREATE TABLE problem_review_submissions (
    id text PRIMARY KEY CHECK (id ~ '^[a-f0-9]{32}$'),
    owner_subject_id text NOT NULL REFERENCES identity_credentials(id),
    request_key text COLLATE "C" NOT NULL CHECK (request_key ~ '^[A-Za-z0-9_-]{16,128}$'),
    draft_id text NOT NULL REFERENCES problem_drafts(id),
    source_id text COLLATE "C" NOT NULL CHECK (octet_length(source_id) BETWEEN 1 AND 128),
    content_version text NOT NULL CHECK (content_version ~ '^[a-f0-9]{64}$'),
    import_revision text NOT NULL CHECK (import_revision ~ '^[a-f0-9]{64}$'),
    source_lock_sha256 text NOT NULL CHECK (source_lock_sha256 ~ '^[a-f0-9]{64}$'),
    reference_request_id text NOT NULL REFERENCES problem_reference_worker_reports(request_id),
    formal_plan_sha256 text NOT NULL CHECK (formal_plan_sha256 ~ '^[a-f0-9]{64}$'),
    profile_sha256 text NOT NULL CHECK (profile_sha256 ~ '^[a-f0-9]{64}$'),
    evidence_sha256 text NOT NULL CHECK (evidence_sha256 ~ '^[a-f0-9]{64}$'),
    formal_verdict text NOT NULL CHECK (formal_verdict IN ('AC', 'WA', 'CE', 'RE', 'TLE', 'MLE', 'OLE')),
    differential_plan_sha256 text NOT NULL CHECK (differential_plan_sha256 = '' OR differential_plan_sha256 ~ '^[a-f0-9]{64}$'),
    differential_outcome text NOT NULL CHECK (differential_outcome IN ('', 'passed', 'mismatch')
        AND (differential_outcome = '' OR differential_plan_sha256 <> '')),
    auth_generation bigint NOT NULL CHECK (auth_generation > 0),
    recovery_id text NOT NULL CHECK (recovery_id ~ '^[a-f0-9]{32}$'),
    author_revision bigint NOT NULL CHECK (author_revision > 0),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(created_at)),
    UNIQUE(owner_subject_id, request_key)
);
CREATE INDEX problem_review_submissions_draft ON problem_review_submissions(draft_id);
CREATE INDEX problem_review_submissions_reference ON problem_review_submissions(reference_request_id);

-- +goose Down
-- Stop relevant readers and writers before rollback. The direct FK parents
-- precede the new table; do not lock the reference request table (the Worker
-- may read it before taking identity locks).
SET LOCAL lock_timeout = '5s';
LOCK TABLE identity_credentials, problem_drafts, problem_reference_worker_reports,
    problem_review_submissions IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM problem_review_submissions) THEN
        RAISE EXCEPTION 'cannot discard review submissions; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE problem_review_submissions;

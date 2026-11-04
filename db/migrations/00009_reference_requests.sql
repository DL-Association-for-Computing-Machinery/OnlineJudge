-- +goose Up
-- Empty by default: only an explicit maintenance operation enables admission.
CREATE TABLE problem_reference_request_policy (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    global_limit bigint NOT NULL CHECK (global_limit BETWEEN 1 AND 1024),
    owner_limit bigint NOT NULL CHECK (owner_limit BETWEEN 1 AND global_limit)
);

CREATE TABLE problem_reference_requests (
    id text PRIMARY KEY CHECK (id ~ '^[a-f0-9]{32}$'),
    owner_subject_id text NOT NULL REFERENCES identity_credentials(id),
    request_key text COLLATE "C" NOT NULL CHECK (request_key ~ '^[A-Za-z0-9_-]{16,128}$'),
    draft_id text NOT NULL REFERENCES problem_drafts(id),
    source_id text COLLATE "C" NOT NULL CHECK (octet_length(source_id) BETWEEN 1 AND 128),
    content_version text NOT NULL CHECK (content_version ~ '^[a-f0-9]{64}$'),
    import_revision text NOT NULL CHECK (import_revision ~ '^[a-f0-9]{64}$'),
    source_lock_sha256 text NOT NULL CHECK (source_lock_sha256 ~ '^[a-f0-9]{64}$'),
    identity_protocol text NOT NULL CHECK (identity_protocol = 'oj.reference-request.v1'),
    profile_encoding bytea NOT NULL CHECK (octet_length(profile_encoding) BETWEEN 1 AND 16384),
    profile_sha256 text NOT NULL CHECK (profile_sha256 ~ '^[a-f0-9]{64}$'),
    auth_generation bigint NOT NULL CHECK (auth_generation > 0),
    recovery_id text NOT NULL CHECK (recovery_id ~ '^[a-f0-9]{32}$'),
    author_revision bigint NOT NULL CHECK (author_revision > 0),
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'cancelled')),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    cancelled_at timestamptz,
    CHECK ((state = 'queued' AND cancelled_at IS NULL) OR (state = 'cancelled' AND cancelled_at IS NOT NULL)),
    UNIQUE(owner_subject_id, request_key)
);

-- Bounded queued counts; terminal history stays outside this index. Each queued
-- insert/cancel writes it once; it is not an execution-slot or history quota.
CREATE INDEX problem_reference_requests_queued_owner
    ON problem_reference_requests(owner_subject_id, id) WHERE state = 'queued';

-- +goose Down
-- Table locks follow first access, including FK trigger removal's parent locks:
-- authority reads credentials before the request lookup, then draft and policy.
-- Locking requests before credentials or draft before requests can deadlock an
-- in-flight caller. No identity rows are changed and no history is discarded.
LOCK TABLE identity_credentials, problem_reference_requests, problem_drafts,
    problem_reference_request_policy IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM problem_reference_requests)
       OR EXISTS (SELECT 1 FROM problem_reference_request_policy) THEN
        RAISE EXCEPTION 'cannot discard reference requests or configured admission; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE problem_reference_requests;
DROP TABLE problem_reference_request_policy;

-- +goose Up
CREATE TABLE identity_problem_authors (
    subject_id text PRIMARY KEY REFERENCES identity_credentials(id),
    enabled boolean NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0)
);
CREATE TABLE identity_problem_author_audit (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    subject_id text NOT NULL REFERENCES identity_problem_authors(subject_id),
    previous_revision bigint NOT NULL CHECK (previous_revision >= 0),
    revision bigint NOT NULL CHECK (revision = previous_revision + 1),
    enabled boolean NOT NULL,
    maintenance_operator text NOT NULL CHECK (octet_length(maintenance_operator) BETWEEN 1 AND 80),
    reason text NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 512),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(subject_id, revision)
);
CREATE TABLE problem_drafts (
    id text PRIMARY KEY CHECK (id ~ '^[a-f0-9]{32}$'),
    owner_subject_id text NOT NULL REFERENCES identity_credentials(id),
    request_key text COLLATE "C" NOT NULL CHECK (request_key ~ '^[A-Za-z0-9_-]{16,128}$'),
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[a-f0-9]{64}$'),
    source_id text COLLATE "C" NOT NULL CHECK (octet_length(source_id) BETWEEN 1 AND 128),
    content_version text NOT NULL CHECK (content_version ~ '^[a-f0-9]{64}$'),
    state text NOT NULL DEFAULT 'private-draft' CHECK (state = 'private-draft'),
    validation_state text NOT NULL DEFAULT 'not-run' CHECK (validation_state = 'not-run'),
    document bytea NOT NULL CHECK (octet_length(document) BETWEEN 1 AND 1048576),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(owner_subject_id, request_key)
);
CREATE TABLE problem_draft_materials (
    draft_id text NOT NULL REFERENCES problem_drafts(id),
    logical_name text COLLATE "C" NOT NULL CHECK (octet_length(logical_name) BETWEEN 1 AND 240),
    sha256 text NOT NULL CHECK (sha256 ~ '^[a-f0-9]{64}$'),
    byte_size bigint NOT NULL CHECK (byte_size BETWEEN 0 AND 134217728),
    PRIMARY KEY(draft_id, logical_name)
);

-- +goose Down
-- Authorization and drafts share recovery -> account -> role -> draft order.
LOCK TABLE identity_credentials, identity_problem_authors, identity_problem_author_audit,
    problem_drafts, problem_draft_materials IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM identity_problem_authors)
       OR EXISTS (SELECT 1 FROM identity_problem_author_audit)
       OR EXISTS (SELECT 1 FROM problem_drafts) THEN
        RAISE EXCEPTION 'cannot discard author grants, audit or private drafts; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE problem_draft_materials;
DROP TABLE problem_drafts;
DROP TABLE identity_problem_author_audit;
DROP TABLE identity_problem_authors;

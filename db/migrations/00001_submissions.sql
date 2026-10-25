-- +goose Up
CREATE TABLE submission_materials (
    version_id text PRIMARY KEY,
    manifest_sha256 text NOT NULL CHECK (manifest_sha256 ~ '^[a-f0-9]{64}$'),
    manifest jsonb NOT NULL CHECK (jsonb_typeof(manifest) = 'object'),
    UNIQUE (version_id, manifest_sha256)
);

CREATE TABLE submissions (
    id text PRIMARY KEY,
    subject_id text NOT NULL,
    context_id text NOT NULL,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 128),
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[a-f0-9]{64}$'),
    material_version text NOT NULL,
    manifest_sha256 text NOT NULL,
    language text NOT NULL CHECK (language = 'gnu++17'),
    source_code text NOT NULL CHECK (octet_length(source_code) BETWEEN 1 AND 65536),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (subject_id, context_id, idempotency_key),
    FOREIGN KEY (material_version, manifest_sha256)
        REFERENCES submission_materials (version_id, manifest_sha256)
);

-- Scheduling state is separate from verdict. No worker may fabricate a verdict
-- merely because a submission was durably accepted.
CREATE TABLE judge_tasks (
    submission_id text PRIMARY KEY REFERENCES submissions(id),
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'finished', 'failed')),
    judge_generation bigint NOT NULL DEFAULT 1 CHECK (judge_generation > 0),
    recovery_generation bigint NOT NULL DEFAULT 1 CHECK (recovery_generation > 0),
    attempt_number integer NOT NULL DEFAULT 0 CHECK (attempt_number >= 0),
    verdict text CHECK (verdict IN ('AC', 'WA', 'CE', 'RE', 'TLE', 'MLE', 'OLE', 'SE')),
    CHECK ((state = 'finished' AND verdict IS NOT NULL) OR (state <> 'finished' AND verdict IS NULL))
);

-- +goose Down
-- Experimental schema only. Refuse destructive rollback once data is accepted.
-- The migration advisory lock does not serialize API writes. Wait for them
-- before taking the emptiness snapshot, and block new writers until commit.
LOCK TABLE submissions IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM submissions) THEN
        RAISE EXCEPTION 'cannot roll back accepted submissions; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE judge_tasks;
DROP TABLE submissions;
DROP TABLE submission_materials;

-- +goose Up
SET LOCAL lock_timeout = '5s';
CREATE TABLE submission_rejudges (
    operator text NOT NULL CHECK (octet_length(operator) BETWEEN 1 AND 63),
    request_key text NOT NULL CHECK (request_key ~ '^[A-Za-z0-9_-]{16,128}$'),
    submission_id text NOT NULL REFERENCES submissions(id),
    previous_generation bigint NOT NULL CHECK (previous_generation BETWEEN 1 AND 9223372036854775806),
    material_version text NOT NULL,
    manifest_sha256 text NOT NULL,
    reason text NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 240),
    requested_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (operator, request_key),
    UNIQUE (submission_id, previous_generation),
    FOREIGN KEY (material_version, manifest_sha256) REFERENCES submission_materials(version_id, manifest_sha256)
);

-- +goose Down
SET LOCAL lock_timeout = '5s';
LOCK TABLE submission_rejudges IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM submission_rejudges) THEN
        RAISE EXCEPTION 'cannot discard rejudge audit records; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE submission_rejudges;

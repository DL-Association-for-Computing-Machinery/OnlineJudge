-- +goose Up
CREATE TABLE identity_platform_admin_history (
    subject_id text NOT NULL REFERENCES identity_credentials(id),
    revision bigint NOT NULL CHECK (revision > 0),
    enabled boolean NOT NULL,
    maintenance_operator text NOT NULL CHECK (octet_length(maintenance_operator) BETWEEN 1 AND 80),
    reason text NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 512),
    changed_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(changed_at)),
    PRIMARY KEY (subject_id, revision)
);

-- +goose Down
SET LOCAL lock_timeout = '5s';
LOCK TABLE identity_credentials, identity_platform_admin_history IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM identity_platform_admin_history) THEN
        RAISE EXCEPTION 'cannot discard platform administrator history; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE identity_platform_admin_history;

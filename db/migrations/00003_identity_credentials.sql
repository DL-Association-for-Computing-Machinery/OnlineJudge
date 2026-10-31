-- +goose Up
CREATE TABLE identity_credentials (
    id text PRIMARY KEY CHECK (id ~ '^[a-f0-9]{32}$'),
    enabled boolean NOT NULL,
    credential_version bigint NOT NULL DEFAULT 1 CHECK (credential_version > 0),
    auth_generation bigint NOT NULL DEFAULT 1 CHECK (auth_generation > 0),
    format_version integer NOT NULL CHECK (format_version = 1),
    algorithm text NOT NULL CHECK (algorithm = 'argon2id'),
    argon_version integer NOT NULL CHECK (argon_version = 19),
    memory_kib bigint NOT NULL CHECK (memory_kib BETWEEN 8 AND 4294967295),
    iterations bigint NOT NULL CHECK (iterations BETWEEN 1 AND 4294967295),
    parallelism smallint NOT NULL CHECK (parallelism BETWEEN 1 AND 255),
    salt_bytes integer NOT NULL CHECK (salt_bytes BETWEEN 8 AND 64),
    digest_bytes integer NOT NULL CHECK (digest_bytes BETWEEN 16 AND 64),
    salt bytea NOT NULL CHECK (octet_length(salt) = salt_bytes),
    digest bytea NOT NULL CHECK (octet_length(digest) = digest_bytes),
    CHECK (memory_kib >= 8 * parallelism AND memory_kib % (4 * parallelism) = 0)
);

-- +goose Down
-- Guard both credential initialization and accepted work. Acquire identity
-- first, then submissions; no password computation runs under these locks.
LOCK TABLE identity_credentials, submissions IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM identity_credentials) OR EXISTS (SELECT 1 FROM submissions) THEN
        RAISE EXCEPTION 'cannot roll back stored identities or accepted submissions; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE identity_credentials;

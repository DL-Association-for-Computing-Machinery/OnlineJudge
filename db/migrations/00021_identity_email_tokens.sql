-- +goose Up
SET LOCAL lock_timeout = '5s';
CREATE TABLE identity_email_tokens (
    id text PRIMARY KEY CHECK (id ~ '^[a-f0-9]{32}$'),
    subject_id text NOT NULL REFERENCES identity_personal_emails(subject_id),
    purpose text NOT NULL CHECK (purpose IN ('activation', 'password-recovery')),
    recovery_id text NOT NULL CHECK (recovery_id ~ '^[a-f0-9]{32}$'),
    auth_generation bigint NOT NULL CHECK (auth_generation > 0),
    digest bytea NOT NULL UNIQUE CHECK (octet_length(digest) = 32),
    issued_at timestamptz NOT NULL CHECK (isfinite(issued_at)),
    expires_at timestamptz NOT NULL CHECK (isfinite(expires_at)),
    CHECK (expires_at - issued_at = CASE purpose
        WHEN 'activation' THEN interval '24 hours' ELSE interval '30 minutes' END)
);
CREATE INDEX identity_email_tokens_recent
    ON identity_email_tokens(subject_id, purpose, issued_at DESC);
CREATE INDEX identity_email_tokens_live
    ON identity_email_tokens(subject_id, purpose, recovery_id, auth_generation, expires_at);

CREATE TABLE identity_email_payloads (
    token_id text PRIMARY KEY REFERENCES identity_email_tokens(id),
    key_id text NOT NULL CHECK (key_id ~ '^[a-z0-9_-]{1,16}$'),
    ciphertext bytea NOT NULL CHECK (octet_length(ciphertext) = 60),
    send_until timestamptz NOT NULL CHECK (isfinite(send_until))
);

-- +goose Down
SET LOCAL lock_timeout = '5s';
LOCK TABLE identity_personal_emails, identity_email_tokens, identity_email_payloads IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM identity_email_tokens) OR EXISTS (SELECT 1 FROM identity_email_payloads) THEN
        RAISE EXCEPTION 'cannot discard issued email tokens; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE identity_email_payloads;
DROP TABLE identity_email_tokens;

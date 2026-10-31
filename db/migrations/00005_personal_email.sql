-- +goose Up
CREATE TABLE identity_personal_emails (
    subject_id text PRIMARY KEY REFERENCES identity_credentials(id),
    email_key text COLLATE "C" NOT NULL UNIQUE,
    original_email text COLLATE "C" NOT NULL,
    email_verified boolean NOT NULL DEFAULT false,
    CHECK (octet_length(original_email) BETWEEN 3 AND 254),
    CHECK (original_email !~ '[^!-~]'),
    CHECK (email_key = lower(original_email COLLATE "C"))
);

-- +goose Down
-- Match initialization's order: dropping the FK also locks credentials when
-- removing its internal triggers. Locking email first could deadlock a first write.
LOCK TABLE identity_credentials, identity_personal_emails IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM identity_personal_emails) THEN
        RAISE EXCEPTION 'cannot roll back personal email mappings; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE identity_personal_emails;

-- +goose Up
CREATE TABLE identity_session_recovery (
    singleton boolean NOT NULL DEFAULT true PRIMARY KEY CHECK (singleton),
    generation text NOT NULL DEFAULT '' CHECK (generation = '' OR generation ~ '^[a-f0-9]{32}$'),
    enabled boolean NOT NULL DEFAULT false,
    CHECK (NOT enabled OR generation <> '')
);
-- Bootstrap closed. Only explicit maintenance may generate and open an epoch.
INSERT INTO identity_session_recovery (singleton) VALUES (true);

-- +goose Down
-- Keep the global -> subject -> accepted-work ordering used by session guards.
LOCK TABLE identity_session_recovery, identity_credentials, submissions IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF (SELECT count(*) FROM identity_session_recovery) <> 1
       OR NOT EXISTS (SELECT 1 FROM identity_session_recovery WHERE singleton AND generation = '' AND NOT enabled)
       OR EXISTS (SELECT 1 FROM identity_credentials)
       OR EXISTS (SELECT 1 FROM submissions) THEN
        RAISE EXCEPTION 'cannot discard initialized recovery or retained business data; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE identity_session_recovery;

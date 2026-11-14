-- +goose Up
ALTER TABLE submissions DROP CONSTRAINT submissions_language_check,
    ADD CONSTRAINT submissions_language_check CHECK (language IN ('gnu++17', 'c17'));

-- +goose Down
-- Adding the old constraint refuses rollback while any C17 history exists.
ALTER TABLE submissions DROP CONSTRAINT submissions_language_check,
    ADD CONSTRAINT submissions_language_check CHECK (language = 'gnu++17');

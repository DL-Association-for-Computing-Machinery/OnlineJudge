-- +goose Up
ALTER TABLE submissions DROP CONSTRAINT submissions_language_check,
    ADD CONSTRAINT submissions_language_check CHECK (language IN ('gnu++17', 'c17', 'gnu++20'));

-- +goose Down
-- The old constraint rejects rollback if GNU++20 history exists.
ALTER TABLE submissions DROP CONSTRAINT submissions_language_check,
    ADD CONSTRAINT submissions_language_check CHECK (language IN ('gnu++17', 'c17'));

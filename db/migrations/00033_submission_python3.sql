-- +goose Up
ALTER TABLE submissions DROP CONSTRAINT submissions_language_check,
    ADD CONSTRAINT submissions_language_check CHECK (language IN ('gnu++17', 'c17', 'gnu++20', 'java21', 'python3'));

-- +goose Down
-- Preserve existing Python history by refusing to narrow the constraint.
ALTER TABLE submissions DROP CONSTRAINT submissions_language_check,
    ADD CONSTRAINT submissions_language_check CHECK (language IN ('gnu++17', 'c17', 'gnu++20', 'java21'));

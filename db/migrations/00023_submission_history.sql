-- +goose Up
CREATE INDEX submissions_owner_history ON submissions (subject_id, context_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX submissions_owner_history;

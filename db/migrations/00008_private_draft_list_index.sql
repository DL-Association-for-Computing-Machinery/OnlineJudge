-- +goose Up
CREATE INDEX problem_drafts_owner_created_id_idx ON problem_drafts(owner_subject_id, created_at DESC, id COLLATE "C" DESC);

-- +goose Down
DROP INDEX problem_drafts_owner_created_id_idx;

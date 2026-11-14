-- +goose Up
SET LOCAL lock_timeout = '5s';
CREATE INDEX problem_review_assignment_discovery
    ON problem_review_assignment_history (reviewer_subject_id, reviewer_revision, review_id COLLATE "C")
    WHERE reviewer_subject_id IS NOT NULL;

-- +goose Down
SET LOCAL lock_timeout = '5s';
DROP INDEX problem_review_assignment_discovery;

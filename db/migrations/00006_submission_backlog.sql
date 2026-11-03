-- +goose Up
-- Configuration is an explicit administrative operation, not an API default.
CREATE TABLE submission_backlog_policy (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    global_limit bigint NOT NULL CHECK (global_limit > 0),
    subject_limit bigint NOT NULL CHECK (subject_limit > 0 AND subject_limit <= global_limit)
);

CREATE INDEX judge_tasks_unfinished ON judge_tasks(submission_id)
    WHERE state IN ('queued', 'running');

-- +goose Down
-- Acceptance reads submissions before locking the policy. Preserve that order.
LOCK TABLE submissions, submission_backlog_policy IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM submissions) OR EXISTS (SELECT 1 FROM submission_backlog_policy) THEN
        RAISE EXCEPTION 'cannot remove configured admission or accepted submissions; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP INDEX judge_tasks_unfinished;
DROP TABLE submission_backlog_policy;

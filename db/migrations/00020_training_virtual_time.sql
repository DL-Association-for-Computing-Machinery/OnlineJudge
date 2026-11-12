-- +goose Up
SET LOCAL lock_timeout = '5s';
CREATE TABLE training_queue_clock (
    version_id text PRIMARY KEY REFERENCES submission_materials(version_id),
    floor bigint NOT NULL DEFAULT 0 CHECK (floor >= 0)
);
INSERT INTO training_queue_clock (version_id) SELECT version_id FROM submission_materials;
CREATE SEQUENCE training_enqueue_order AS bigint;
ALTER TABLE judge_tasks ADD COLUMN queue_version text REFERENCES training_queue_clock(version_id),
    ADD COLUMN virtual_finish bigint, ADD COLUMN enqueue_order bigint;

WITH active AS (
    SELECT s.id, s.material_version,
        row_number() OVER (PARTITION BY s.material_version, s.subject_id ORDER BY s.created_at, s.id) AS rank,
        row_number() OVER (ORDER BY s.created_at, s.id) AS enqueue_order
    FROM judge_tasks t JOIN submissions s ON s.id = t.submission_id
    WHERE t.state IN ('queued', 'running')
)
UPDATE judge_tasks t SET queue_version = active.material_version,
    virtual_finish = active.rank, enqueue_order = active.enqueue_order
FROM active WHERE t.submission_id = active.id;

-- Include requeued attempts: they were already claimed under the old scheduler.
UPDATE training_queue_clock c SET floor = seen.rank FROM (
    SELECT queue_version, max(virtual_finish) AS rank FROM judge_tasks
    WHERE queue_version IS NOT NULL AND (state = 'running' OR attempt_number > 0)
    GROUP BY queue_version
) seen WHERE c.version_id = seen.queue_version;

SELECT setval('training_enqueue_order', GREATEST(COALESCE(max(enqueue_order), 0), 1), max(enqueue_order) IS NOT NULL)
FROM judge_tasks;

ALTER TABLE judge_tasks ADD CONSTRAINT judge_tasks_queue_order CHECK (
    (queue_version IS NOT NULL AND virtual_finish IS NOT NULL AND virtual_finish > 0
        AND enqueue_order IS NOT NULL AND enqueue_order > 0)
    OR (queue_version IS NULL AND virtual_finish IS NULL AND enqueue_order IS NULL AND state IN ('finished', 'failed'))
);
CREATE INDEX judge_tasks_training_queue ON judge_tasks(queue_version, virtual_finish, enqueue_order, submission_id)
    WHERE state = 'queued';

-- +goose Down
SET LOCAL lock_timeout = '5s';
LOCK TABLE judge_tasks IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM judge_tasks WHERE state IN ('queued', 'running')) THEN
        RAISE EXCEPTION 'cannot discard active training queue order; use a forward migration or drain the queue';
    END IF;
END $$;
-- +goose StatementEnd
DROP INDEX judge_tasks_training_queue;
ALTER TABLE judge_tasks DROP CONSTRAINT judge_tasks_queue_order,
    DROP COLUMN queue_version, DROP COLUMN virtual_finish, DROP COLUMN enqueue_order;
DROP SEQUENCE training_enqueue_order;
DROP TABLE training_queue_clock;

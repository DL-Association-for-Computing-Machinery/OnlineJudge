-- +goose Up
CREATE TABLE problem_reference_dispatch_history (
    request_id text PRIMARY KEY REFERENCES problem_reference_requests(id),
    first_claim_at timestamptz CHECK (isfinite(first_claim_at))
);

-- A pre-existing plan could already have run under the overwritten singleton
-- reservation. NULL marks that uncertainty; never admit it to automatic dispatch.
INSERT INTO problem_reference_dispatch_history(request_id, first_claim_at)
SELECT request_id, NULL FROM problem_reference_expected_plans;

-- The additional predicate keeps this index out of the global owner-count
-- plan; the dispatch query names it explicitly alongside its exact source.
CREATE INDEX problem_reference_requests_queued_source
ON problem_reference_requests(source_id,id COLLATE "C") WHERE state = 'queued' AND source_id <> '';

-- +goose Down
LOCK TABLE problem_reference_requests, problem_reference_dispatch_history IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM problem_reference_dispatch_history) THEN
        RAISE EXCEPTION 'cannot discard reference dispatch history; use a forward migration';
    END IF;
END $$;
-- +goose StatementEnd
DROP INDEX problem_reference_requests_queued_source;
DROP TABLE problem_reference_dispatch_history;

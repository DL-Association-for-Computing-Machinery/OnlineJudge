-- name: ReferenceExpectedPlan :one
SELECT plan_encoding, plan_sha256 FROM problem_reference_expected_plans WHERE request_id = @request_id;

-- name: InsertReferenceExpectedPlan :exec
INSERT INTO problem_reference_expected_plans(request_id, plan_encoding, plan_sha256)
VALUES (@request_id, @plan_encoding, @plan_sha256);

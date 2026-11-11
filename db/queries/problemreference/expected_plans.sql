-- name: ReferenceExpectedPlan :one
SELECT plan_encoding, plan_sha256 FROM problem_reference_expected_plans WHERE request_id = @request_id;

-- name: ReferenceConvolutionExpectedPlan :one
SELECT plan_encoding, plan_sha256, differential_encoding, differential_sha256
FROM problem_reference_expected_plans
WHERE request_id = @request_id;

-- name: InsertReferenceExpectedPlan :exec
INSERT INTO problem_reference_expected_plans(request_id, plan_encoding, plan_sha256)
VALUES (@request_id, @plan_encoding, @plan_sha256);

-- name: InsertReferencePlanWithDifferential :exec
INSERT INTO problem_reference_expected_plans(request_id, plan_encoding, plan_sha256, differential_encoding, differential_sha256)
VALUES (@request_id, @plan_encoding, @plan_sha256, sqlc.narg('differential_encoding'), sqlc.narg('differential_sha256'));

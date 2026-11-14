-- name: ReviewApprovalByReviewerKey :one
SELECT * FROM problem_review_approvals WHERE reviewer_subject_id = @reviewer_subject_id AND request_key = @request_key;

-- name: HasReviewApprovalForMaterial :one
SELECT EXISTS (
    SELECT 1
    FROM problem_review_approvals AS approval
    JOIN LATERAL (
        SELECT revision FROM problem_review_assignment_history
        WHERE review_id = approval.review_id ORDER BY revision DESC LIMIT 1
    ) AS assignment ON assignment.revision = approval.assignment_revision
    JOIN LATERAL (
        SELECT revision, enabled FROM identity_problem_reviewer_history
        WHERE subject_id = approval.reviewer_subject_id ORDER BY revision DESC LIMIT 1
    ) AS role ON role.revision = approval.reviewer_revision AND role.enabled
    WHERE approval.material_sha256 = @material_sha256 AND approval.policy_version = @policy_version
);

-- name: InsertReviewApproval :one
INSERT INTO problem_review_approvals (
    review_id, assignment_revision, reviewer_subject_id, reviewer_revision,
    request_key, material_sha256, snapshot_sha256, policy_version, reason,
    auth_generation, recovery_id
) VALUES (
    @review_id, @assignment_revision, @reviewer_subject_id, @reviewer_revision,
    @request_key, @material_sha256, @snapshot_sha256, @policy_version, @reason,
    @auth_generation, @recovery_id
)
ON CONFLICT DO NOTHING
RETURNING *;

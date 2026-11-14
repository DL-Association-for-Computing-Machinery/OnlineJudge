-- name: DiscoverReviewAssignments :many
-- Seek past each distinct review ID without scanning repeated changes of that
-- item. A 21st candidate signals continuation even on a page of stale entries.
WITH RECURSIVE candidates (review_id, position) AS (
    SELECT sqlc.arg(after_id)::text COLLATE "C", 0
    UNION ALL
    SELECT next_assignment.review_id, candidates.position + 1
    FROM candidates
    CROSS JOIN LATERAL (
        SELECT a.review_id COLLATE "C" AS review_id
        FROM problem_review_assignment_history a
        WHERE a.reviewer_subject_id = sqlc.arg(reviewer_id)::text
          AND a.reviewer_revision = sqlc.arg(role_revision)::bigint
          AND a.review_id COLLATE "C" > candidates.review_id
        ORDER BY a.review_id COLLATE "C" ASC
        LIMIT 1
    ) next_assignment
    WHERE candidates.position < 21
)
SELECT s.id AS review_id, c.position, s.source_id, s.import_revision,
       latest.revision AS assignment_revision,
       COALESCE(latest.reviewer_subject_id = sqlc.arg(reviewer_id)::text
           AND latest.reviewer_revision = sqlc.arg(role_revision)::bigint
           AND s.owner_subject_id <> sqlc.arg(reviewer_id)::text, false)::boolean AS is_current
FROM candidates c
JOIN problem_review_submissions s ON s.id = c.review_id COLLATE "default"
CROSS JOIN LATERAL (
    SELECT a.reviewer_subject_id, a.reviewer_revision, a.revision
    FROM problem_review_assignment_history a
    WHERE a.review_id = c.review_id COLLATE "default"
    ORDER BY a.revision DESC
    LIMIT 1
) latest
WHERE c.position > 0
ORDER BY c.position;

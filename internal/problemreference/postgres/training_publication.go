package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

// TrainingPublication is the frozen execution binding of the current local
// publication. The execution admission and registration are protected by FKs.
type TrainingPublication struct {
	Source, ImportRevision, Plan, MaterialVersion, MaterialDigest string
}

// ResolveTrainingPublication reads the latest revision before applying version
// and policy predicates. An older matching publication must never be selected.
// The caller supplies its transaction or read connection.
func ResolveTrainingPublication(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id, version string) (TrainingPublication, error) {
	var p TrainingPublication
	err := q.QueryRow(ctx, `SELECT r.source_id, p.import_revision, p.formal_plan_sha256, p.execution_version, p.execution_sha256
		FROM (
			SELECT * FROM problem_publications WHERE problem_id=$1 ORDER BY revision DESC LIMIT 1
		) p JOIN problem_review_submissions r ON r.id=p.review_id
		JOIN problem_review_approvals a ON a.review_id=p.review_id AND a.assignment_revision=p.assignment_revision
		WHERE p.version=$2 AND a.policy_version=$3
			AND p.import_revision=r.import_revision AND p.formal_plan_sha256=r.formal_plan_sha256`,
		id, version, problemreference.LocalReviewPolicyVersion).
		Scan(&p.Source, &p.ImportRevision, &p.Plan, &p.MaterialVersion, &p.MaterialDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return TrainingPublication{}, problemreference.ErrNotFound
	}
	return p, err
}

package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/jackc/pgx/v5"
)

// AllowLocalTraining authorizes the exact displayed version against the
// current local publication and its FK-bound execution admission.
func (s *ReviewStore) AllowLocalTraining(ctx context.Context, tx pgx.Tx, problemID, version string) error {
	if s == nil || s.policy() != problemreference.LocalReviewPolicyVersion {
		return submission.ErrMaterial
	}
	id, plan := s.local.ReviewBinding()
	_, execution, digest := s.local.Binding()
	var present int
	err := tx.QueryRow(ctx, `SELECT 1 FROM (
		SELECT * FROM problem_publications WHERE problem_id=$1 ORDER BY revision DESC LIMIT 1
	) p JOIN problem_review_submissions r ON r.id=p.review_id
	JOIN problem_review_approvals a ON a.review_id=p.review_id AND a.assignment_revision=p.assignment_revision
	WHERE p.version=$2 AND a.policy_version=$3 AND r.source_id=$4
		AND p.import_revision=$5 AND r.import_revision=$5 AND p.formal_plan_sha256=$6 AND r.formal_plan_sha256=$6
		AND p.execution_version=$7 AND p.execution_sha256=$8`, problemID, version, problemreference.LocalReviewPolicyVersion,
		id.SourceID, id.ImportRevision, plan, execution, digest).Scan(&present)
	if errors.Is(err, pgx.ErrNoRows) {
		return submission.ErrNotFound
	}
	return err
}

// WithLocalExecution creates a store scoped to one explicit local run.
func (s *ReviewStore) WithLocalExecution(a problemvalidation.LocalExecutionAdmission) (*ReviewStore, error) {
	if s == nil || !a.ValidateManifest() || s.policy() != problemreference.ReviewPolicyVersion {
		return nil, problemreference.ErrInvalid
	}
	next := *s
	next.local = a
	return &next, nil
}

func (s *ReviewStore) policy() string {
	if proof, _, _ := s.local.Binding(); proof != "" {
		return problemreference.LocalReviewPolicyVersion
	}
	return problemreference.ReviewPolicyVersion
}

// RegisterLocalExecution requires the already registered, exact execution
// manifest and never writes an approval.
func (s *ReviewStore) RegisterLocalExecution(ctx context.Context) error {
	if s == nil || ctx == nil || !s.local.ValidateManifest() {
		return problemreference.ErrInvalid
	}
	_, version, digest := s.local.Binding()
	store, err := submissionpg.New(s.pool, version, s.local.Manifest())
	if err != nil {
		return err
	}
	if err := store.CheckRegisteredMaterial(ctx); err != nil {
		return err
	}
	id, plan := s.local.ReviewBinding()
	_, err = s.pool.Exec(ctx, `INSERT INTO problem_execution_admissions
		(import_revision,formal_plan_sha256,execution_version,execution_sha256)
		VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, id.ImportRevision, plan, version, digest)
	if err != nil {
		return storageError{err}
	}
	return nil
}

// PrepareLocalPublication loads the frozen plan from the review's own reference
// request. PublishProblem independently rechecks the decision and publisher.
func (s *ReviewStore) PrepareLocalPublication(ctx context.Context, drafts ReviewDraftReader, review, publicID string) (problem.PublicationMaterialPlan, problem.ReferencePlan, error) {
	var empty problem.PublicationMaterialPlan
	var plan problem.ReferencePlan
	if s == nil || ctx == nil || drafts == nil || !identity.ValidID(review) || !identity.ValidID(publicID) ||
		s.policy() != problemreference.LocalReviewPolicyVersion {
		return empty, plan, problemreference.ErrInvalid
	}
	// The draft adapter takes shared row locks while reading its inventory.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return empty, plan, storageError{err}
	}
	defer rollback(tx)
	var owner, draftID string
	var raw []byte
	id, digest := s.local.ReviewBinding()
	err = tx.QueryRow(ctx, `SELECT r.owner_subject_id,r.draft_id,p.plan_encoding
		FROM problem_review_submissions r JOIN problem_reference_expected_plans p ON r.reference_request_id=p.request_id
		WHERE r.id=$1 AND r.import_revision=$2 AND r.formal_plan_sha256=$3 AND p.plan_sha256=$3`,
		review, id.ImportRevision, digest).Scan(&owner, &draftID, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, plan, problemreference.ErrNotFound
	}
	if err != nil {
		return empty, plan, storageError{err}
	}
	draft, err := drafts(ctx, tx, draftID, owner)
	if err != nil {
		return empty, plan, storageError{err}
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		return empty, problem.ReferencePlan{}, storageError{err}
	}
	prepared, err := draft.PlanPublicationMaterials(publicID, draftID, owner, id, plan, digest)
	if err != nil {
		return empty, problem.ReferencePlan{}, err
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return empty, problem.ReferencePlan{}, err
	}
	return prepared, plan, nil
}

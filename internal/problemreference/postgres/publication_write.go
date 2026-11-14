package postgres

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/jackc/pgx/v5"
)

// PublisherAuthority locks the publisher and reviewer accounts in the same tx.
type PublisherAuthority interface {
	LockPublisher(context.Context, pgx.Tx, identity.ProblemActor, int64, string) error
}

// PublicationIntent is an internal maintenance contract. The problem ID,
// sealed artifact and Plan must come from trusted preparation, never a client.
type PublicationIntent struct {
	Materials            problem.PublicationMaterials
	ReviewID, ReviewerID string
	ExecutionVersion     string
	ExecutionSHA256      string
	RequestKey           string
	AssignmentRevision   int64
	PublisherRevision    int64
	ExpectedRevision     int64
}

// PublishProblem verifies objects before acquiring the business locks. The
// trusted caller must hold the shared material budget and keep the objects
// pinned through commit. No HTTP route supplies this operation or admissions.
func (s *ReviewStore) PublishProblem(ctx context.Context, authority PublisherAuthority, actor identity.ProblemActor,
	objects problem.ReferenceObjectReader, plan problem.ReferencePlan, i PublicationIntent) (CatalogEntry, error) {
	var zero CatalogEntry
	if s == nil || s.pool == nil || ctx == nil || authority == nil || !actor.Valid() ||
		!identity.ValidID(i.ReviewID) || !identity.ValidID(i.ReviewerID) || !problem.ValidKey(i.RequestKey) ||
		i.AssignmentRevision < 1 || i.PublisherRevision < 1 || i.ExpectedRevision < 0 || i.ExpectedRevision == math.MaxInt64 ||
		i.ExecutionVersion == "" || len(i.ExecutionVersion) > 128 || !problem.ValidImportRevision(i.ExecutionSHA256) {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	m := i.Materials
	if proof, version, execution := s.local.Binding(); proof != "" && (i.ExecutionVersion != version || i.ExecutionSHA256 != execution) {
		return zero, problemreference.ErrReviewChanged
	}
	if err := problem.VerifySealedPublicationMaterials(ctx, objects, m, plan); err != nil {
		return zero, err
	}
	display, err := problem.ReadSealedPublicationStatement(ctx, objects, m)
	if err != nil {
		return zero, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, storageError{err}
	}
	defer rollback(tx)
	if err := authority.LockPublisher(ctx, tx, actor, i.PublisherRevision, i.ReviewerID); err != nil {
		return zero, err
	}
	q := referencedb.New(tx)
	key := referencedb.PublicationByPublisherKeyParams{PublisherID: actor.SubjectID, RequestKey: i.RequestKey}
	params := referencedb.InsertProblemPublicationParams{
		ProblemID: m.ProblemID, Revision: i.ExpectedRevision + 1, Version: m.Version, Title: display.Title,
		StatementSha256: m.Statement.SHA256, StatementSize: m.Statement.Size, ImportRevision: m.Import.ImportRevision,
		FormalPlanSha256: m.FormalPlan, ExecutionVersion: i.ExecutionVersion, ExecutionSha256: i.ExecutionSHA256,
		ReviewID: i.ReviewID, AssignmentRevision: i.AssignmentRevision, PublisherID: actor.SubjectID,
		PublisherRevision: i.PublisherRevision, RequestKey: i.RequestKey,
	}
	old, err := q.PublicationByPublisherKey(ctx, key)
	if err == nil {
		if err := s.checkPublicationScope(ctx, q, old.ReviewID, old.AssignmentRevision); err != nil {
			return zero, err
		}
		return finishPublication(ctx, tx, old, params, false)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return zero, storageError{err}
	}
	receipt, err := s.LockPublicationReview(ctx, tx, i.ReviewID, i.AssignmentRevision, i.ReviewerID)
	if err != nil {
		return zero, err
	}
	review, err := q.ReviewSubmissionForShare(ctx, i.ReviewID)
	if err != nil {
		return zero, storageError{err}
	}
	if receipt.Binding.ImportRevision != m.Import.ImportRevision || receipt.Binding.FormalPlanSHA256 != m.FormalPlan ||
		review.DraftID == m.ProblemID || review.SourceID != m.Import.SourceID ||
		review.ContentVersion != m.Import.Version || review.SourceLockSha256 != m.Import.SourceLockSHA256 {
		return zero, problemreference.ErrReviewChanged
	}
	current, err := q.LatestProblemPublication(ctx, m.ProblemID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return zero, storageError{err}
	}
	if current.Revision != i.ExpectedRevision {
		return zero, problemreference.ErrConflict
	}
	if current.Revision > 0 {
		previous, err := q.CatalogProblem(ctx, referencedb.CatalogProblemParams{
			ProblemID: m.ProblemID, LocalOnly: s.policy() == problemreference.LocalReviewPolicyVersion})
		if err != nil {
			return zero, storageError{err}
		}
		owner, err := q.PublishedVersionOwner(ctx, referencedb.PublishedVersionOwnerParams{ProblemID: m.ProblemID, Revision: current.Revision})
		if err != nil {
			return zero, storageError{err}
		}
		if previous.Revision != current.Revision || previous.SourceID != m.Import.SourceID || owner != review.OwnerSubjectID {
			return zero, problemreference.ErrConflict
		}
	}
	row, err := q.InsertProblemPublication(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		row, err = q.PublicationByPublisherKey(ctx, key)
		if errors.Is(err, pgx.ErrNoRows) {
			return zero, problemreference.ErrConflict
		}
		if err == nil {
			if err := s.checkPublicationScope(ctx, q, row.ReviewID, row.AssignmentRevision); err != nil {
				return zero, err
			}
			return finishPublication(ctx, tx, row, params, false)
		}
	}
	if err != nil {
		return zero, storageError{err}
	}
	return finishPublication(ctx, tx, row, params, true)
}

func (s *ReviewStore) checkPublicationScope(ctx context.Context, q *referencedb.Queries, review string, revision int64) error {
	a, err := q.ReviewApprovalByAssignment(ctx, referencedb.ReviewApprovalByAssignmentParams{ReviewID: review, AssignmentRevision: revision})
	if err != nil {
		return storageError{err}
	}
	if (a.PolicyVersion == problemreference.LocalReviewPolicyVersion) != (s.policy() == problemreference.LocalReviewPolicyVersion) {
		return problemreference.ErrNotFound
	}
	return nil
}

func finishPublication(ctx context.Context, tx pgx.Tx, row referencedb.ProblemPublication, i referencedb.InsertProblemPublicationParams, wrote bool) (CatalogEntry, error) {
	if row.ProblemID != i.ProblemID || row.Revision != i.Revision || row.Version != i.Version || row.Title != i.Title ||
		row.StatementSha256 != i.StatementSha256 || row.StatementSize != i.StatementSize || row.ImportRevision != i.ImportRevision ||
		row.FormalPlanSha256 != i.FormalPlanSha256 || row.ExecutionVersion != i.ExecutionVersion || row.ExecutionSha256 != i.ExecutionSha256 ||
		row.ReviewID != i.ReviewID || row.AssignmentRevision != i.AssignmentRevision ||
		row.PublisherID != i.PublisherID || row.PublisherRevision != i.PublisherRevision || row.RequestKey != i.RequestKey {
		return CatalogEntry{}, problemreference.ErrConflict
	}
	if err := commitTx(ctx, tx, wrote); err != nil {
		return CatalogEntry{}, err
	}
	return CatalogEntry{ID: row.ProblemID, Version: row.Version, Title: row.Title}, nil
}

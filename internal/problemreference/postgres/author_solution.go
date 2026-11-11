package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AuthorSolutionStore uses the owner-scoped intake and immutable draft metadata.
type AuthorSolutionStore struct {
	base   *Store
	drafts ReviewDraftReader
}

// NewAuthorSolutionStore binds a metadata-only draft reader to the author store.
func NewAuthorSolutionStore(base *Store, drafts ReviewDraftReader) (*AuthorSolutionStore, error) {
	if base == nil || drafts == nil {
		return nil, problemreference.ErrInvalid
	}
	return &AuthorSolutionStore{base: base, drafts: drafts}, nil
}

// AuthorSolutionSnapshot returns metadata only; no object I/O holds PG locks.
func (s *AuthorSolutionStore) AuthorSolutionSnapshot(ctx context.Context, actor identity.ProblemActor, key string) (problemreference.AuthorSolutionSnapshot, error) {
	var result problemreference.AuthorSolutionSnapshot
	err := s.withSolution(ctx, actor, key, func(_ context.Context, _ *referencedb.Queries, snapshot problemreference.AuthorSolutionSnapshot) (bool, error) {
		result = snapshot
		return false, nil
	})
	if err != nil {
		return problemreference.AuthorSolutionSnapshot{}, err
	}
	return result, nil
}

// ConfirmAuthorSolution rechecks the exact selection before the idempotent write.
func (s *AuthorSolutionStore) ConfirmAuthorSolution(ctx context.Context, actor identity.ProblemActor, key string, expected problemreference.AuthorSolutionSnapshot) (problemreference.SolutionConfirmation, error) {
	var result problemreference.SolutionConfirmation
	err := s.withSolution(ctx, actor, key, func(ctx context.Context, q *referencedb.Queries, snapshot problemreference.AuthorSolutionSnapshot) (bool, error) {
		if !expected.SameSelection(snapshot) {
			return false, problemreference.ErrReviewChanged
		}
		row, err := q.InsertSolutionConfirmation(ctx, referencedb.InsertSolutionConfirmationParams{
			DraftID: snapshot.DraftID, ImportRevision: snapshot.Binding.ImportRevision, FormalPlanSha256: snapshot.Binding.FormalPlanSHA256,
			ReviewID: snapshot.ReviewID, OwnerSubjectID: snapshot.Owner, AuthorRevision: snapshot.AuthorRevision,
			AuthGeneration: actor.AuthGeneration, RecoveryID: actor.RecoveryID,
		})
		wrote := err == nil
		if errors.Is(err, pgx.ErrNoRows) {
			// READ COMMITTED must take a new snapshot after waiting on the
			// unique index; an insert's snapshot may not see the winner.
			row, err = q.SolutionConfirmation(ctx, confirmationParams(snapshot.DraftID, snapshot.Binding))
		}
		if err != nil {
			return false, storageError{err}
		}
		result, err = solutionReceipt(row, snapshot, key)
		return wrote, err
	})
	if err != nil {
		return problemreference.SolutionConfirmation{}, err
	}
	return result, nil
}

// AuthorSolutionConfirmation reads a durable receipt without reading objects.
func (s *AuthorSolutionStore) AuthorSolutionConfirmation(ctx context.Context, actor identity.ProblemActor, key string) (problemreference.SolutionConfirmation, error) {
	var result problemreference.SolutionConfirmation
	err := s.withSolution(ctx, actor, key, func(ctx context.Context, q *referencedb.Queries, snapshot problemreference.AuthorSolutionSnapshot) (bool, error) {
		row, err := q.SolutionConfirmation(ctx, confirmationParams(snapshot.DraftID, snapshot.Binding))
		if errors.Is(err, pgx.ErrNoRows) {
			return false, problemreference.ErrNotFound
		}
		if err != nil {
			return false, storageError{err}
		}
		result, err = solutionReceipt(row, snapshot, key)
		return false, err
	})
	if err != nil {
		return problemreference.SolutionConfirmation{}, err
	}
	return result, nil
}

func (s *AuthorSolutionStore) withSolution(ctx context.Context, actor identity.ProblemActor, key string,
	fn func(context.Context, *referencedb.Queries, problemreference.AuthorSolutionSnapshot) (bool, error),
) error {
	if s == nil || s.base == nil || s.drafts == nil || ctx == nil || !actor.Valid() || !problem.ValidKey(key) {
		return problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, authority, err := s.base.begin(ctx, actor)
	if err != nil {
		return err
	}
	defer rollback(tx)
	q := referencedb.New(tx)
	row, err := q.ReviewSubmissionByOwnerKey(ctx, referencedb.ReviewSubmissionByOwnerKeyParams{Owner: actor.SubjectID, RequestKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return problemreference.ErrNotFound
	}
	if err != nil {
		return storageError{err}
	}
	if _, err := readReviewReceipt(row, actor.SubjectID, key); err != nil {
		return err
	}
	current, err := s.base.access.LockOwnDraftIdentity(ctx, tx, actor, row.DraftID)
	if err != nil {
		return err
	}
	if current.ID != row.DraftID || current.Owner != actor.SubjectID || current.SourceID != row.SourceID ||
		current.ImportRevision != row.ImportRevision || current.Version != row.ContentVersion {
		return problemreference.ErrReviewChanged
	}
	frozen, err := loadHistoricalReviewEvidence(ctx, q, actor.SubjectID, reviewEvidence(row))
	if err != nil {
		return storageError{err}
	}
	var plan problem.ReferencePlan
	if err := json.Unmarshal(frozen.expected.Encoding, &plan); err != nil {
		return storageError{err}
	}
	draft, err := s.drafts(ctx, tx, row.DraftID, actor.SubjectID)
	if err != nil {
		return storageError{err}
	}
	manifest, err := draft.PrepareReviewMaterials(row.DraftID, actor.SubjectID, frozen.bound, plan)
	if err != nil {
		return storageError{err}
	}
	binding := problemreference.SolutionBinding{ImportRevision: row.ImportRevision, FormalPlanSHA256: row.FormalPlanSha256}
	snapshot := problemreference.AuthorSolutionSnapshot{Owner: actor.SubjectID, DraftID: row.DraftID, ReviewID: row.ID,
		AuthorRevision: authority.RoleRevision, Binding: binding, Manifest: manifest}
	wrote, err := fn(ctx, q, snapshot)
	if err != nil {
		return err
	}
	if err := commitTx(ctx, tx, wrote); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil && wrote {
		return errors.Join(problemreference.ErrCommitUnknown, err)
	}
	return ctx.Err()
}

func confirmationParams(draft string, binding problemreference.SolutionBinding) referencedb.SolutionConfirmationParams {
	return referencedb.SolutionConfirmationParams{DraftID: draft, ImportRevision: binding.ImportRevision, FormalPlanSha256: binding.FormalPlanSHA256}
}

func solutionReceipt(row referencedb.ProblemSolutionConfirmation, s problemreference.AuthorSolutionSnapshot, key string) (problemreference.SolutionConfirmation, error) {
	if !validSolutionConfirmation(row, s.Owner, s.DraftID, s.Binding) {
		return problemreference.SolutionConfirmation{}, storageError{problemreference.ErrInvalid}
	}
	return problemreference.SolutionConfirmation{Owner: s.Owner, IntakeKey: key, SolutionBinding: s.Binding, ConfirmedAt: row.ConfirmedAt.Time}, nil
}

func validSolutionConfirmation(row referencedb.ProblemSolutionConfirmation, owner, draft string, binding problemreference.SolutionBinding) bool {
	return row.DraftID == draft && row.OwnerSubjectID == owner && row.ImportRevision == binding.ImportRevision &&
		row.FormalPlanSha256 == binding.FormalPlanSHA256 && identity.ValidID(row.ReviewID) &&
		identity.ValidID(row.RecoveryID) && row.AuthGeneration > 0 && row.AuthorRevision > 0 &&
		row.ConfirmedAt.Valid && row.ConfirmedAt.InfinityModifier == pgtype.Finite && !row.ConfirmedAt.Time.IsZero()
}

package postgres

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/jackc/pgx/v5"
)

// ApproveReview recomputes eligibility in the same transaction as the decision.
// A matching digest alone can never authorize an approval.
func (s *ReviewStore) ApproveReview(ctx context.Context, actor identity.ProblemActor, id string, i problemreference.ApprovalIntent) (problemreference.ReviewApprovalReceipt, error) {
	if !i.Valid() {
		return problemreference.ReviewApprovalReceipt{}, problemreference.ErrInvalid
	}
	return s.approval(ctx, actor, id, i.RequestKey, &i)
}

// ReviewApprovalReceipt reads the original request key under the original,
// still-current assignment. A missing receipt does not prove a write rolled back.
func (s *ReviewStore) ReviewApprovalReceipt(ctx context.Context, actor identity.ProblemActor, id, key string) (problemreference.ReviewApprovalReceipt, error) {
	return s.approval(ctx, actor, id, key, nil)
}

func (s *ReviewStore) approval(ctx context.Context, actor identity.ProblemActor, id, key string, intent *problemreference.ApprovalIntent) (problemreference.ReviewApprovalReceipt, error) {
	var zero problemreference.ReviewApprovalReceipt
	if !s.validRead(ctx, actor, id) || !problem.ValidKey(key) {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, storageError{err}
	}
	defer rollback(tx)
	item, err := s.lockAssignedReview(ctx, tx, actor, id)
	if err != nil {
		return zero, err
	}
	q := referencedb.New(tx)
	digest := item.materialDigest()
	material := hex.EncodeToString(digest[:])
	lookup := referencedb.ReviewApprovalByReviewerKeyParams{ReviewerSubjectID: actor.SubjectID, RequestKey: key}
	if intent != nil {
		if err := lockReviewMaterial(ctx, q, digest); err != nil {
			return zero, storageError{err}
		}
	}
	row, err := q.ReviewApprovalByReviewerKey(ctx, lookup)
	wrote := false
	if intent != nil && errors.Is(err, pgx.ErrNoRows) {
		if intent.AssignmentRevision != item.evidence.AssignmentRevision {
			return zero, problemreference.ErrReviewChanged
		}
		snapshot, snapshotErr := loadReviewEligibilitySnapshot(ctx, q, item)
		if snapshotErr != nil {
			return zero, snapshotErr
		}
		if err := problemreference.CheckReviewApprovalSnapshot(actor, snapshot, intent.SnapshotSHA256); err != nil {
			return zero, err
		}
		approved, approvalErr := hasCurrentReviewApproval(ctx, q, material)
		if approvalErr != nil {
			return zero, approvalErr
		}
		if approved {
			return zero, problemreference.ErrReviewAlreadyApproved
		}
		row, err = q.InsertReviewApproval(ctx, referencedb.InsertReviewApprovalParams{
			ReviewID: id, AssignmentRevision: intent.AssignmentRevision, ReviewerSubjectID: actor.SubjectID,
			ReviewerRevision: item.roleRevision, RequestKey: key, MaterialSha256: material, SnapshotSha256: intent.SnapshotSHA256,
			PolicyVersion: problemreference.ReviewPolicyVersion, Reason: intent.Reason, AuthGeneration: actor.AuthGeneration, RecoveryID: actor.RecoveryID,
		})
		wrote = err == nil
		if errors.Is(err, pgx.ErrNoRows) {
			row, err = q.ReviewApprovalByReviewerKey(ctx, lookup)
			if errors.Is(err, pgx.ErrNoRows) {
				return zero, problemreference.ErrConflict
			}
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	if row.ReviewID != id || row.AssignmentRevision != item.evidence.AssignmentRevision {
		if intent == nil {
			return zero, problemreference.ErrNotFound
		}
		return zero, problemreference.ErrConflict
	}
	result, err := checkedApprovalReceipt(row, item, material, key)
	if err != nil {
		return zero, err
	}
	if intent != nil && result.ApprovalIntent != *intent {
		return zero, problemreference.ErrConflict
	}
	if err := commitTx(ctx, tx, wrote); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		if wrote {
			return zero, errors.Join(problemreference.ErrCommitUnknown, err)
		}
		return zero, err
	}
	return result, nil
}

// Call under the material lock shared by approve and return. A historical
// approval cannot prevent a newly assigned reviewer from making a decision.
func hasCurrentReviewApproval(ctx context.Context, q *referencedb.Queries, material string) (bool, error) {
	approved, err := q.HasReviewApprovalForMaterial(ctx, referencedb.HasReviewApprovalForMaterialParams{
		MaterialSha256: material, PolicyVersion: problemreference.ReviewPolicyVersion,
	})
	if err != nil {
		return false, storageError{err}
	}
	return approved, nil
}

func checkedApprovalReceipt(row referencedb.ProblemReviewApproval, item assignedReview, material, key string) (problemreference.ReviewApprovalReceipt, error) {
	e := item.evidence
	r := problemreference.ReviewApprovalReceipt{
		Binding: problemreference.ReviewApprovalBinding{ReviewID: row.ReviewID, Reviewer: row.ReviewerSubjectID,
			AssignmentRevision: row.AssignmentRevision, ReviewerRevision: row.ReviewerRevision,
			ImportRevision: e.ImportRevision, FormalPlanSHA256: e.FormalPlanSHA256, EvidenceSHA256: e.EvidenceSHA256, PolicyVersion: row.PolicyVersion},
		ApprovalIntent: problemreference.ApprovalIntent{RequestKey: row.RequestKey, AssignmentRevision: row.AssignmentRevision,
			SnapshotSHA256: row.SnapshotSha256, Reason: row.Reason},
		ApprovedAt: row.ApprovedAt.Time,
	}
	if !r.Valid() || row.ReviewerSubjectID != e.Reviewer || row.ReviewerRevision != item.roleRevision ||
		row.MaterialSha256 != material || row.RequestKey != key || !row.ApprovedAt.Valid || row.ApprovedAt.InfinityModifier != 0 {
		return problemreference.ReviewApprovalReceipt{}, storageError{problemreference.ErrInvalid}
	}
	return r, nil
}

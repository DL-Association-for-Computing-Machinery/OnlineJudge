package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/jackc/pgx/v5"
)

// All fields except the final optional plan are fixed-length hex digests or
// subject IDs. The namespace is stable across processes and restarts.
func reviewMaterialDigest(owner, revision, formal, profile, evidence, differential string) [32]byte {
	return sha256.Sum256([]byte("onlinejudge/review-material/v1:" + owner + revision + formal + profile + evidence + differential))
}

func (a assignedReview) materialDigest() [32]byte {
	e := a.evidence
	return reviewMaterialDigest(a.owner, e.ImportRevision, e.FormalPlanSHA256, e.ProfileSHA256, e.EvidenceSHA256, e.ComparisonPlan)
}

func lockReviewMaterial(ctx context.Context, q *referencedb.Queries, digest [32]byte) error {
	// A truncated collision only serializes two transactions. All decisions
	// and readbacks are looked up using the complete digest.
	return q.LockReviewMaterial(ctx, int64(binary.BigEndian.Uint64(digest[:8])))
}

// ReturnReview records a single terminal return for the frozen material
// identity. Existing duplicate intake rows cannot record another decision.
func (s *ReviewStore) ReturnReview(ctx context.Context, actor identity.ProblemActor, id string, i problemreference.ReturnIntent) (problemreference.ReviewReturnReceipt, error) {
	var zero problemreference.ReviewReturnReceipt
	if !s.validRead(ctx, actor, id) || !i.Valid() {
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
	if err := lockReviewMaterial(ctx, q, digest); err != nil {
		return zero, storageError{err}
	}
	material := hex.EncodeToString(digest[:])
	key := referencedb.ReviewReturnByReviewerKeyParams{ReviewerSubjectID: actor.SubjectID, RequestKey: i.RequestKey}
	row, err := q.ReviewReturnByReviewerKey(ctx, key)
	wrote := false
	if errors.Is(err, pgx.ErrNoRows) {
		if item.evidence.AssignmentRevision != i.AssignmentRevision ||
			item.evidence.ImportRevision != i.ImportRevision || item.evidence.EvidenceSHA256 != i.EvidenceSHA256 {
			return zero, problemreference.ErrReviewChanged
		}
		if _, err := q.ReviewReturnByMaterial(ctx, material); err == nil {
			return zero, problemreference.ErrReviewReturned
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return zero, storageError{err}
		}
		for _, policy := range []string{problemreference.ReviewPolicyVersion, problemreference.LocalReviewPolicyVersion} {
			approved, err := hasCurrentReviewApproval(ctx, q, material, policy)
			if err != nil {
				return zero, err
			}
			if approved {
				return zero, problemreference.ErrReviewAlreadyApproved
			}
		}
		row, err = q.InsertReviewReturn(ctx, referencedb.InsertReviewReturnParams{
			ReviewID: id, MaterialSha256: material, ReviewerSubjectID: actor.SubjectID, RequestKey: i.RequestKey,
			Reason: i.Reason, AuthGeneration: actor.AuthGeneration, RecoveryID: actor.RecoveryID,
			ReviewerRevision: item.roleRevision, AssignmentRevision: i.AssignmentRevision,
		})
		wrote = err == nil
		if errors.Is(err, pgx.ErrNoRows) {
			// Another material can race on the reviewer/key index.
			row, err = q.ReviewReturnByReviewerKey(ctx, key)
		}
	}
	if err != nil {
		return zero, storageError{err}
	}
	if row.ReviewID != id {
		return zero, problemreference.ErrConflict
	}
	receipt, err := returnReceipt(row, item, actor.SubjectID, i.RequestKey, material)
	if err != nil {
		return zero, err
	}
	if receipt.ReturnIntent != i {
		return zero, problemreference.ErrConflict
	}
	if err := commitTx(ctx, tx, wrote); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, errors.Join(problemreference.ErrCommitUnknown, err)
	}
	return receipt, nil
}

// ReviewReturnReceipt reads one reviewer-owned original key under current
// assignment. A missing receipt is not proof that an earlier write rolled back.
func (s *ReviewStore) ReviewReturnReceipt(ctx context.Context, actor identity.ProblemActor, id, key string) (problemreference.ReviewReturnReceipt, error) {
	var zero problemreference.ReviewReturnReceipt
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
	row, err := referencedb.New(tx).ReviewReturnByReviewerKey(ctx, referencedb.ReviewReturnByReviewerKeyParams{ReviewerSubjectID: actor.SubjectID, RequestKey: key})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.ReviewID != id) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	digest := item.materialDigest()
	r, err := returnReceipt(row, item, actor.SubjectID, key, hex.EncodeToString(digest[:]))
	if err != nil {
		return zero, err
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return r, nil
}

func returnReceipt(row referencedb.ProblemReviewReturn, item assignedReview, reviewer, key, material string) (problemreference.ReviewReturnReceipt, error) {
	r := problemreference.ReviewReturnReceipt{Reviewer: row.ReviewerSubjectID, ReviewID: row.ReviewID,
		ReturnIntent: problemreference.ReturnIntent{RequestKey: row.RequestKey, AssignmentRevision: row.AssignmentRevision,
			ImportRevision: item.evidence.ImportRevision, EvidenceSHA256: item.evidence.EvidenceSHA256, Reason: row.Reason},
		ReturnedAt: row.ReturnedAt.Time}
	if !r.Valid() || r.Reviewer != reviewer || r.RequestKey != key || r.ReviewID != item.evidence.ReviewID ||
		row.MaterialSha256 != material || !row.ReturnedAt.Valid || row.ReturnedAt.InfinityModifier != 0 {
		return problemreference.ReviewReturnReceipt{}, storageError{problemreference.ErrInvalid}
	}
	return r, nil
}

// AuthorReviewReturn returns the material-level decision for one owned intake,
// even if a different duplicate intake was the one originally returned.
func (s *Store) AuthorReviewReturn(ctx context.Context, actor identity.ProblemActor, key string) (problemreference.ReviewReturnStatus, error) {
	var zero problemreference.ReviewReturnStatus
	if ctx == nil || !problem.ValidKey(key) {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, _, err := s.begin(ctx, actor)
	if err != nil {
		return zero, err
	}
	defer rollback(tx)
	q := referencedb.New(tx)
	intake, err := q.ReviewSubmissionByOwnerKey(ctx, referencedb.ReviewSubmissionByOwnerKeyParams{Owner: actor.SubjectID, RequestKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	if _, err := readReviewReceipt(intake, actor.SubjectID, key); err != nil {
		return zero, err
	}
	digest := reviewMaterialDigest(actor.SubjectID, intake.ImportRevision, intake.FormalPlanSha256, intake.ProfileSha256,
		intake.EvidenceSha256, intake.DifferentialPlanSha256)
	material := hex.EncodeToString(digest[:])
	row, err := q.ReviewReturnByMaterial(ctx, material)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	r := problemreference.ReviewReturnStatus{Owner: actor.SubjectID, IntakeKey: key, ReviewID: intake.ID, ImportRevision: intake.ImportRevision,
		EvidenceSHA256: intake.EvidenceSha256, Reason: row.Reason, ReturnedAt: row.ReturnedAt.Time}
	if !r.Valid() || row.MaterialSha256 != material || !row.ReturnedAt.Valid || row.ReturnedAt.InfinityModifier != 0 {
		return zero, storageError{problemreference.ErrInvalid}
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return r, nil
}

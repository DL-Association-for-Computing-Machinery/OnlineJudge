package postgres

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/jackc/pgx/v5"
)

// LockPublicationReview rechecks an exact saved approval under the caller's
// publication transaction. The caller must provide a bounded context, use
// read committed, and retain the locks until the publication write commits.
// On any error the caller must roll back; this method never ends the transaction.
// The expected reviewer is for planning account locks; the ledger must match
// it. The caller must separately check publish authority and execution materials.
func (s *ReviewStore) LockPublicationReview(ctx context.Context, tx pgx.Tx, id string, assignmentRevision int64, expectedReviewer string) (problemreference.ReviewApprovalReceipt, error) {
	var zero problemreference.ReviewApprovalReceipt
	if s == nil || s.auth == nil || ctx == nil || tx == nil ||
		!identity.ValidID(id) || assignmentRevision < 1 || !identity.ValidID(expectedReviewer) {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Grant and assignment updates append history under a parent lock. An
	// older repeatable-read snapshot could miss those rows even after waiting.
	var isolation string
	if err := tx.QueryRow(ctx, "SHOW transaction_isolation").Scan(&isolation); err != nil {
		return zero, storageError{err}
	}
	if isolation != "read committed" {
		return zero, problemreference.ErrInvalid
	}
	q := referencedb.New(tx)
	row, err := q.ReviewApprovalByAssignment(ctx, referencedb.ReviewApprovalByAssignmentParams{
		ReviewID: id, AssignmentRevision: assignmentRevision,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	// The immutable row supplies the reviewer and generations. Neither the
	// publisher's identity nor caller-provided reviewer claims may replace them.
	actor := identity.ProblemActor{SubjectID: row.ReviewerSubjectID,
		SessionVersion: identity.SessionVersion{AuthGeneration: row.AuthGeneration, RecoveryID: row.RecoveryID}}
	if !actor.Valid() || row.ReviewID != id || row.AssignmentRevision != assignmentRevision {
		return zero, storageError{problemreference.ErrInvalid}
	}
	if actor.SubjectID != expectedReviewer {
		return zero, problemreference.ErrReviewChanged
	}
	item, err := s.lockAssignedReview(ctx, tx, actor, id)
	if err != nil {
		return zero, err
	}
	evidence := item.evidence
	if assignmentRevision != evidence.AssignmentRevision || row.ReviewerRevision != item.roleRevision {
		return zero, problemreference.ErrReviewChanged
	}
	digest := item.materialDigest()
	if err := lockReviewMaterial(ctx, q, digest); err != nil {
		return zero, storageError{err}
	}
	receipt, err := checkedApprovalReceipt(row, item, hex.EncodeToString(digest[:]), row.RequestKey)
	if err != nil {
		return zero, err
	}
	snapshot, err := s.loadReviewEligibilitySnapshot(ctx, tx, q, item)
	if err != nil {
		return zero, err
	}
	if receipt.Binding.PolicyVersion != s.policy() {
		return zero, problemreference.ErrReviewChanged
	}
	if err := problemreference.CheckReviewApprovalSnapshot(actor, snapshot, receipt.SnapshotSHA256); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return receipt, nil
}

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

// RecordStatementConfirmation rechecks the frozen selection inside the
// transaction that writes. Object and session I/O have already completed.
func (s *ReviewMaterialStore) RecordStatementConfirmation(ctx context.Context, actor identity.ProblemActor, id string, i problemreference.StatementConfirmationIntent, before problemreference.ReviewMaterialSnapshot) (problemreference.StatementConfirmationReceipt, error) {
	var zero problemreference.StatementConfirmationReceipt
	if s == nil || !s.reviews.validRead(ctx, actor, id) || !i.Valid() {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.reviews.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, storageError{err}
	}
	defer rollback(tx)
	item, err := s.reviews.lockAssignedReview(ctx, tx, actor, id)
	if err != nil {
		return zero, err
	}
	if !i.Matches(item.evidence) || item.evidence != before.Evidence || item.owner != before.Owner || item.roleRevision != before.RoleRevision {
		return zero, problemreference.ErrReviewChanged
	}
	q := referencedb.New(tx)
	digest := item.materialDigest()
	if err := lockReviewMaterial(ctx, q, digest); err != nil {
		return zero, storageError{err}
	}
	key := referencedb.StatementConfirmationByKeyParams{ReviewerSubjectID: actor.SubjectID, RequestKey: i.RequestKey}
	row, err := q.StatementConfirmationByKey(ctx, key)
	wrote := false
	if errors.Is(err, pgx.ErrNoRows) {
		row, wrote, err = s.insertStatementConfirmation(ctx, tx, q, actor, item, i, before, hex.EncodeToString(digest[:]))
	}
	if err != nil {
		return zero, err
	}
	if row.ReviewID != id || row.AssignmentRevision != i.AssignmentRevision {
		return zero, problemreference.ErrConflict
	}
	receipt, err := checkedStatementConfirmation(row, item)
	if err != nil {
		return zero, err
	}
	if receipt.StatementConfirmationIntent != i {
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

func (s *ReviewMaterialStore) insertStatementConfirmation(ctx context.Context, tx pgx.Tx, q *referencedb.Queries, actor identity.ProblemActor, item assignedReview, i problemreference.StatementConfirmationIntent, before problemreference.ReviewMaterialSnapshot, material string) (referencedb.ProblemStatementConfirmation, bool, error) {
	var zero referencedb.ProblemStatementConfirmation
	if _, err := q.ReviewReturnByMaterial(ctx, material); err == nil {
		return zero, false, problemreference.ErrReviewReturned
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return zero, false, storageError{err}
	}
	current, err := s.materialSnapshot(ctx, tx, item)
	if err != nil {
		return zero, false, err
	}
	if !current.Manifest.Equal(before.Manifest) {
		return zero, false, problemreference.ErrReviewChanged
	}
	row, err := q.InsertStatementConfirmation(ctx, referencedb.InsertStatementConfirmationParams{
		ReviewID: item.evidence.ReviewID, AssignmentRevision: i.AssignmentRevision,
		ReviewerSubjectID: actor.SubjectID, ReviewerRevision: item.roleRevision, RequestKey: i.RequestKey,
		ImportRevision: i.ImportRevision, EvidenceSha256: i.EvidenceSHA256, ChecklistVersion: i.ChecklistVersion,
		AuthGeneration: actor.AuthGeneration, RecoveryID: actor.RecoveryID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// A confirmation on a different material can race on the reviewer/key index.
		row, err = q.StatementConfirmationByKey(ctx, referencedb.StatementConfirmationByKeyParams{
			ReviewerSubjectID: actor.SubjectID, RequestKey: i.RequestKey})
		if errors.Is(err, pgx.ErrNoRows) {
			return zero, false, problemreference.ErrConflict
		}
		return row, false, wrapStatementStoreError(err)
	}
	return row, err == nil, wrapStatementStoreError(err)
}

// StatementConfirmationReceipt reads a key only under the original live assignment.
func (s *ReviewMaterialStore) StatementConfirmationReceipt(ctx context.Context, actor identity.ProblemActor, id, key string) (problemreference.StatementConfirmationReceipt, error) {
	var zero problemreference.StatementConfirmationReceipt
	if s == nil || !s.reviews.validRead(ctx, actor, id) || !problem.ValidKey(key) {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.reviews.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, storageError{err}
	}
	defer rollback(tx)
	item, err := s.reviews.lockAssignedReview(ctx, tx, actor, id)
	if err != nil {
		return zero, err
	}
	row, err := referencedb.New(tx).StatementConfirmationByKey(ctx, referencedb.StatementConfirmationByKeyParams{
		ReviewerSubjectID: actor.SubjectID, RequestKey: key})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (row.ReviewID != id || row.AssignmentRevision != item.evidence.AssignmentRevision)) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	r, err := checkedStatementConfirmation(row, item)
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

func checkedStatementConfirmation(row referencedb.ProblemStatementConfirmation, item assignedReview) (problemreference.StatementConfirmationReceipt, error) {
	r := problemreference.StatementConfirmationReceipt{
		Reviewer: row.ReviewerSubjectID, ReviewID: row.ReviewID, ConfirmedAt: row.ConfirmedAt.Time,
		StatementConfirmationIntent: problemreference.StatementConfirmationIntent{
			RequestKey: row.RequestKey, AssignmentRevision: row.AssignmentRevision, ImportRevision: row.ImportRevision,
			EvidenceSHA256: row.EvidenceSha256, ChecklistVersion: row.ChecklistVersion,
		},
	}
	if !r.Valid() || !r.Matches(item.evidence) || r.ReviewID != item.evidence.ReviewID || r.Reviewer != item.evidence.Reviewer ||
		row.ReviewerRevision != item.roleRevision || !row.ConfirmedAt.Valid || row.ConfirmedAt.InfinityModifier != 0 {
		return problemreference.StatementConfirmationReceipt{}, storageError{problemreference.ErrInvalid}
	}
	return r, nil
}

func wrapStatementStoreError(err error) error {
	if err == nil {
		return nil
	}
	return storageError{err}
}

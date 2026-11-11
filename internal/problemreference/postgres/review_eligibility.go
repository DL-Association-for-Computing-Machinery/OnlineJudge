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

// ReviewEligibilitySnapshot checks authority and the accepted report in one
// short transaction. It reads the shared return status without exposing the decision.
func (s *ReviewStore) ReviewEligibilitySnapshot(ctx context.Context, actor identity.ProblemActor, id string) (problemreference.ReviewEligibilitySnapshot, error) {
	var zero problemreference.ReviewEligibilitySnapshot
	if !s.validRead(ctx, actor, id) {
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
	if _, err := loadHistoricalReviewEvidence(ctx, q, item.owner, item.evidence); err != nil {
		return zero, storageError{err}
	}
	statement, err := q.StatementConfirmationByAssignment(ctx, referencedb.StatementConfirmationByAssignmentParams{
		ReviewID: id, AssignmentRevision: item.evidence.AssignmentRevision,
	})
	statementConfirmed := err == nil
	if statementConfirmed {
		if _, err := checkedStatementConfirmation(statement, item); err != nil {
			return zero, err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return zero, storageError{err}
	}
	// Check returns after confirmations: a return that precedes an observed
	// confirmation can never be missed by this read-committed transaction.
	digest := item.materialDigest()
	_, err = q.ReviewReturnByMaterial(ctx, hex.EncodeToString(digest[:]))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return zero, storageError{err}
	}
	returned := err == nil
	binding := problemreference.SolutionBinding{ImportRevision: item.evidence.ImportRevision, FormalPlanSHA256: item.evidence.FormalPlanSHA256}
	row, err := q.SolutionConfirmation(ctx, confirmationParams(item.evidence.DraftID, binding))
	confirmed := err == nil
	switch {
	case confirmed && !validSolutionConfirmation(row, item.owner, item.evidence.DraftID, binding):
		return zero, storageError{problemreference.ErrInvalid}
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return zero, storageError{err}
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return problemreference.ReviewEligibilitySnapshot{Evidence: item.evidence, Returned: returned, SolutionConfirmed: confirmed,
		StatementConfirmed: statementConfirmed && !returned}, nil
}

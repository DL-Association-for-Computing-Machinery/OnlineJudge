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
	if _, err := loadHistoricalReviewEvidence(ctx, q, item); err != nil {
		return zero, storageError{err}
	}
	digest := item.materialDigest()
	_, err = q.ReviewReturnByMaterial(ctx, hex.EncodeToString(digest[:]))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return zero, storageError{err}
	}
	returned := err == nil
	if err := commitTx(ctx, tx, false); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return problemreference.ReviewEligibilitySnapshot{Evidence: item.evidence, Returned: returned}, nil
}

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/jackc/pgx/v5"
)

// DispatchReference examines at most eight queued rows for one fixed source.
// The scan is only a hint: each actual reservation repeats live authorization
// and checks the history while the request row is locked.
func (c *ReferenceClaimer) DispatchReference(ctx context.Context, node submission.NodeIdentity, input problemreference.DispatchInput) (problemreference.DispatchResult, error) {
	var zero problemreference.DispatchResult
	if c == nil || ctx == nil || !input.Valid() {
		return zero, problemreference.ErrInvalid
	}
	if node.ID != c.nodeID || node.CredentialVersion < 1 {
		return zero, submission.ErrNode
	}
	if _, err := c.resolveProfile(input.SourceID); err != nil {
		return zero, problemreference.ErrClaimUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rows, err := c.dispatchCandidates(ctx, node, input)
	if err != nil {
		return zero, err
	}
	var after string
	for _, row := range rows {
		after = row.ID
		if row.Dispatched || row.PlanSha256 == "" {
			continue
		}
		claim, err := c.claimReference(ctx, node, problemreference.ClaimInput{
			RequestID: row.ID, SourceID: input.SourceID, PlanSHA256: row.PlanSha256,
		}, true)
		if err == nil {
			return problemreference.DispatchResult{Claim: &claim}, nil
		}
		if !errors.Is(err, problemreference.ErrClaimUnavailable) || errors.Is(err, errReferenceReservationBusy) {
			return zero, err
		}
	}
	if err := ctx.Err(); err != nil {
		return zero, storageError{err}
	}
	return problemreference.DispatchResult{After: after}, nil
}

func (c *ReferenceClaimer) dispatchCandidates(ctx context.Context, node submission.NodeIdentity, input problemreference.DispatchInput) ([]referencedb.ReferenceDispatchCandidatesRow, error) {
	tx, err := c.store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, storageError{err}
	}
	defer rollback(tx)
	if err := c.lockNode(ctx, tx, node); err != nil {
		return nil, err
	}
	rows, err := referencedb.New(tx).ReferenceDispatchCandidates(ctx, referencedb.ReferenceDispatchCandidatesParams{
		SourceID: input.SourceID, AfterID: input.After,
	})
	if err != nil {
		return nil, storageError{err}
	}
	// This read holds no durable changes. Release its node lock before the
	// per-candidate transaction takes the full authorization lock sequence.
	return rows, nil
}

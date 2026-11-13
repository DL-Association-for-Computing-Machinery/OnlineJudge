package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
)

// RetireEmailPayload deletes the encrypted mail body by payload ID, leaving
// the token digest and lifetime unchanged. It is idempotent and uses the
// caller-owned transaction. The caller must establish queue ownership and
// terminal state in that transaction; this operation does not authorize them.
func RetireEmailPayload(ctx context.Context, tx pgx.Tx, payloadID string) error {
	if ctx == nil || tx == nil || !identity.ValidID(payloadID) {
		return identity.ErrInvalid
	}
	if err := identitydb.New(tx).DeleteEmailPayload(ctx, payloadID); err != nil {
		// The sender may persist errors in the queue. Keep driver details out.
		return identity.ErrStore
	}
	return nil
}

// EmailPayloadSweep describes one bounded batch; it contains no email or token.
type EmailPayloadSweep struct {
	NextCursor string
	Removed    int64
}

// SweepExpiredEmailPayloads considers at most limit candidates in primary key order and
// removes only payloads whose send window has ended at database time. Locked
// rows are skipped. Pass the returned cursor to the next call; an empty cursor
// marks the end of a pass. Restart from an empty cursor on the next periodic
// pass so skipped or newly expired rows can be reconsidered.
func SweepExpiredEmailPayloads(ctx context.Context, pool *pgxpool.Pool, cursor string, limit int32) (EmailPayloadSweep, error) {
	if ctx == nil || pool == nil || (cursor != "" && !identity.ValidID(cursor)) || limit < 1 || limit > 512 {
		return EmailPayloadSweep{}, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	batch, err := identitydb.New(pool).SweepExpiredEmailPayloads(ctx, identitydb.SweepExpiredEmailPayloadsParams{
		AfterID: cursor, BatchSize: limit,
	})
	if err != nil {
		return EmailPayloadSweep{}, identity.ErrStore
	}
	if batch.Visited < int64(limit) {
		return EmailPayloadSweep{Removed: batch.Removed}, nil
	}
	return EmailPayloadSweep{NextCursor: batch.LastID, Removed: batch.Removed}, nil
}

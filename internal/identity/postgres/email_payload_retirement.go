package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

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

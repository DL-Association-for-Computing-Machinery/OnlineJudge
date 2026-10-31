package sessionexperiment

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const postgresCleanupMaxBatch = 128

var errCleanupBatch = errors.New("cleanup batch must be between 1 and 128")

const postgresCleanupSQL = `WITH expired AS MATERIALIZED (
	SELECT namespace,token FROM session_pg_store_fixture
	WHERE namespace=$1 AND deadline_ms<=$2
	ORDER BY deadline_ms,token LIMIT $3 FOR UPDATE SKIP LOCKED
)
DELETE FROM session_pg_store_fixture s USING expired e
WHERE s.namespace=$1 AND s.namespace=e.namespace AND s.token=e.token AND s.deadline_ms<=$2`

// Only absolute expiry permits physical deletion: idle expiry or revocation
// alone still needs the tombstone to reject a loaded request's later save.
func (s *postgresSessionStore) cleanupExpired(ctx context.Context, batch int) (int64, error) {
	if batch < 1 || batch > postgresCleanupMaxBatch {
		return 0, errCleanupBatch
	}
	// SKIP LOCKED avoids row waits, not DDL/table-lock waits or pool starvation.
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var deleted int64
	err := s.transact(ctx, func(tx pgx.Tx) error {
		var cutoff int64
		if err := tx.QueryRow(ctx, `SELECT floor(extract(epoch FROM clock_timestamp())*1000)::bigint`).Scan(&cutoff); err != nil {
			return err
		}
		var err error
		deleted, err = s.cleanupExpiredTx(ctx, tx, cutoff, batch)
		return err
	})
	if err != nil {
		// A failed COMMIT acknowledgement remains unknown; do not report a
		// successful count, or infer rollback from this zero return value.
		return 0, err
	}
	return deleted, nil
}

// Internal body uses one database-clock cutoff per batch. Its caller validates
// the batch and owns the transaction; tests reuse it for exact-ms boundaries.
// It takes only token locks and never subsequently requests upstream locks.
func (s *postgresSessionStore) cleanupExpiredTx(ctx context.Context, tx pgx.Tx, cutoff int64, batch int) (int64, error) {
	tag, err := tx.Exec(ctx, postgresCleanupSQL, s.namespace, cutoff, batch)
	return tag.RowsAffected(), err
}

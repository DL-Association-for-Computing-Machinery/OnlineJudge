package mailqueue

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SendRate gives all account mail executors one non-accumulating permit
// stream. All replicas must use the same authoritative database and schema.
type SendRate struct{ pool *pgxpool.Pool }

// NewSendRate checks that an operator has configured the shared spacing.
// It does not provision or change the rate.
func NewSendRate(ctx context.Context, pool *pgxpool.Pool) (*SendRate, error) {
	if ctx == nil || pool == nil {
		return nil, errQueue
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var configured bool
	if err := pool.QueryRow(ctx, "SELECT singleton FROM account_mail_send_rate WHERE singleton").Scan(&configured); err != nil || !configured {
		return nil, errQueue
	}
	return &SendRate{pool: pool}, nil
}

// TryTake admits one immediate attempt after a confirmed commit. On denial,
// the caller should defer the job without consuming an SMTP attempt. A
// permit is never refundable or transferable to a later attempt.
func (r *SendRate) TryTake(ctx context.Context) (bool, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return false, errQueue
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, errQueue
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	// Read the clock only after the row lock so queueing does not mint
	// permits using a stale timestamp.
	var configured bool
	if err := tx.QueryRow(ctx, "SELECT singleton FROM account_mail_send_rate WHERE singleton FOR UPDATE").Scan(&configured); err != nil || !configured {
		return false, errQueue
	}
	if _, err := tx.Exec(ctx, "SET LOCAL synchronous_commit = on"); err != nil {
		return false, errQueue
	}
	tag, err := tx.Exec(ctx, `WITH cutoff AS MATERIALIZED (SELECT clock_timestamp() AS now)
		UPDATE account_mail_send_rate SET next_permit_at = cutoff.now + spacing_ms * interval '1 millisecond'
		FROM cutoff WHERE singleton AND next_permit_at <= cutoff.now`)
	if err == nil && tag.RowsAffected() == 0 {
		return false, nil
	}
	if err != nil || tag.RowsAffected() != 1 || tx.Commit(ctx) != nil || ctx.Err() != nil {
		return false, errQueue
	}
	return true, nil
}

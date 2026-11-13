package mailqueue

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

// ErrStaleClaim means this delivery attempt no longer owns the queue row.
var ErrStaleClaim = errors.New("mail job claim is no longer current")

// CompleteTx consumes the current claim and retires its payload in the
// caller's transaction. Use a real River worker context and a transaction
// opened against the same database and search path as its River client
// (which must use the default schema setting). Callers must roll back on
// any error. This function performs no external I/O and does not commit.
// The fence does not cover River's automatic processing of Work results.
func CompleteTx(ctx context.Context, tx pgx.Tx, job *river.Job[Args]) error {
	if ctx == nil || tx == nil || job == nil || job.JobRow == nil ||
		job.State != rivertype.JobStateRunning || job.Queue != Queue || job.Kind != (Args{}).Kind() ||
		job.ID < 1 || job.Attempt < 1 || job.AttemptedAt == nil || len(job.AttemptedBy) == 0 ||
		!identity.ValidID(job.Args.PayloadID) {
		return identity.ErrInvalid
	}
	if _, err := river.ClientFromContextSafely[pgx.Tx](ctx); err != nil {
		return identity.ErrInvalid
	}
	if err := lockClaim(ctx, tx, job); err != nil {
		return err
	}
	updated, err := river.JobCompleteTx[*riverpgxv5.Driver](ctx, tx, job)
	if err != nil || updated.State != rivertype.JobStateCompleted {
		return errQueue
	}
	return identitypg.RetireEmailPayload(ctx, tx, job.Args.PayloadID)
}

func lockClaim(ctx context.Context, tx pgx.Tx, job *river.Job[Args]) error {
	var attempt int
	var attemptedAt time.Time
	var payloadID string
	var attemptedBy []string
	err := tx.QueryRow(ctx, `SELECT attempt, attempted_at, args->>'payload_id', attempted_by
		FROM river_job WHERE id=$1 AND queue=$2 AND kind=$3 AND state='running'
		AND NOT (metadata ? 'cancel_attempted_at') FOR UPDATE`, job.ID, Queue, (Args{}).Kind()).
		Scan(&attempt, &attemptedAt, &payloadID, &attemptedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrStaleClaim
	}
	if err != nil {
		return errQueue
	}
	if attempt != job.Attempt || !attemptedAt.Equal(*job.AttemptedAt) || payloadID != job.Args.PayloadID ||
		len(attemptedBy) == 0 || attemptedBy[len(attemptedBy)-1] != job.AttemptedBy[len(job.AttemptedBy)-1] {
		return ErrStaleClaim
	}
	return nil
}

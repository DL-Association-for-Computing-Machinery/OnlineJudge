package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// AcceptedTrainingSubmission is a receipt observation without account or source data.
type AcceptedTrainingSubmission struct {
	ID              string    `json:"id"`
	AcceptedAt      time.Time `json:"accepted_at"`
	MaterialVersion string    `json:"material_version"`
}

// TrainingAcceptanceObservation enumerates a complete, closed acceptance window
// as visible from one primary database snapshot.
type TrainingAcceptanceObservation struct {
	Schema     string                       `json:"schema"`
	From       time.Time                    `json:"from"`
	Until      time.Time                    `json:"until"`
	ObservedAt time.Time                    `json:"observed_at"`
	Accepted   []AcceptedTrainingSubmission `json:"accepted"`
}

const maxAcceptanceRows = 20_000

const trainingAcceptanceSQL = `
SELECT id, created_at, material_version
FROM submissions
WHERE context_id = $1 AND created_at >= $2 AND created_at < $3
ORDER BY created_at, id
LIMIT $4`

// ObserveTrainingAcceptance returns all personal-training receipts in [from,
// until), without consulting client replies or the mutable judging state.
// The caller must use the primary and quiesce and drain intake before export:
// a transaction still in flight when the snapshot is taken is not visible.
func ObserveTrainingAcceptance(ctx context.Context, pool *pgxpool.Pool, database string, from, until time.Time) (TrainingAcceptanceObservation, error) {
	zero := TrainingAcceptanceObservation{}
	if ctx == nil || pool == nil || database == "" || from.IsZero() || !from.Before(until) || until.Sub(from) > time.Hour {
		return zero, ErrJudgeSnapshotUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return zero, ErrJudgeSnapshotUnavailable
	}
	defer rollback(tx)
	var eligible bool
	var observed time.Time
	err = tx.QueryRow(ctx, `SELECT current_database()::text = $1 AND NOT pg_is_in_recovery(), statement_timestamp()`, database).Scan(&eligible, &observed)
	if err != nil || !eligible || observed.Before(until) {
		return zero, ErrJudgeSnapshotUnavailable
	}
	rows, err := tx.Query(ctx, trainingAcceptanceSQL, submission.PersonalTrainingScope, from, until, maxAcceptanceRows+1)
	if err != nil {
		return zero, ErrJudgeSnapshotUnavailable
	}
	defer rows.Close()
	accepted := []AcceptedTrainingSubmission{}
	for rows.Next() {
		var record AcceptedTrainingSubmission
		if len(accepted) == maxAcceptanceRows || rows.Scan(&record.ID, &record.AcceptedAt, &record.MaterialVersion) != nil {
			return zero, ErrJudgeSnapshotUnavailable
		}
		record.AcceptedAt = record.AcceptedAt.UTC()
		accepted = append(accepted, record)
	}
	if rows.Err() != nil || tx.Commit(ctx) != nil || ctx.Err() != nil {
		return zero, ErrJudgeSnapshotUnavailable
	}
	return TrainingAcceptanceObservation{Schema: "oj.p6-training-acceptance.v1", From: from.UTC(),
		Until: until.UTC(), ObservedAt: observed.UTC(), Accepted: accepted}, nil
}

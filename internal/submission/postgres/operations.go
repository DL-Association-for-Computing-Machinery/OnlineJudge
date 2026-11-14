package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrJudgeSnapshotUnavailable means there was no complete, usable observation.
var ErrJudgeSnapshotUnavailable = errors.New("judge operations snapshot unavailable")

// JudgeOperationsSnapshot exposes aggregates, never individual work or nodes.
// Recent counts are grouped by submission acceptance, not by attempt time.
type JudgeOperationsSnapshot struct {
	Schema                    string     `json:"schema"`
	ObservedAt                time.Time  `json:"observed_at"`
	AcceptedWindowMinutes     int        `json:"accepted_window_minutes"`
	Queued                    int64      `json:"queued"`
	Running                   int64      `json:"running"`
	OldestQueuedAcceptedAt    *time.Time `json:"oldest_queued_accepted_at"`
	RunningWithCurrentLease   int64      `json:"running_with_current_lease"`
	RunningWithoutUsableLease int64      `json:"running_without_usable_lease"`
	ConfiguredEnabledNodes    int64      `json:"configured_enabled_nodes"`
	ConfiguredEnabledSlots    int64      `json:"configured_enabled_slots"`
	RecentlyAccepted          int64      `json:"recently_accepted"`
	RecentWithSystemError     int64      `json:"recent_with_system_error"`
	RecentRecoveryExhausted   int64      `json:"recent_recovery_exhausted"`
}

const judgeOperationsSQL = `
WITH observation AS (SELECT statement_timestamp() AS observed_at),
queue AS (
  SELECT count(*) FILTER (WHERE t.state = 'queued') AS queued,
         count(*) FILTER (WHERE t.state = 'running') AS running,
         min(s.created_at) FILTER (WHERE t.state = 'queued') AS oldest_queued_accepted_at,
         count(*) FILTER (WHERE t.state = 'running' AND EXISTS (
           SELECT 1 FROM judge_attempts a JOIN worker_nodes n ON n.id = a.node_id
           WHERE a.submission_id = t.submission_id AND a.judge_generation = t.judge_generation
             AND a.recovery_generation = t.recovery_generation AND a.attempt_number = t.attempt_number
             AND a.state = 'leased' AND a.lease_until > o.observed_at
             AND n.enabled AND a.credential_version = n.credential_version
         )) AS usable
  FROM judge_tasks t JOIN submissions s ON s.id = t.submission_id CROSS JOIN observation o
  WHERE t.state IN ('queued', 'running')
),
nodes AS (
  SELECT count(*) AS enabled, coalesce(sum(capacity), 0) AS slots FROM worker_nodes WHERE enabled
),
recent AS (
  SELECT count(*) AS accepted,
         count(*) FILTER (WHERE EXISTS (
           SELECT 1 FROM judge_attempts a
           WHERE a.submission_id = t.submission_id AND a.judge_generation = t.judge_generation
             AND a.recovery_generation = t.recovery_generation AND a.state = 'completed'
             AND a.result_payload->>'Verdict' = 'SE'
         )) AS system_error,
         count(*) FILTER (WHERE t.state = 'failed') AS recovery_exhausted
  FROM submissions s JOIN judge_tasks t ON t.submission_id = s.id CROSS JOIN observation o
  WHERE s.created_at > o.observed_at - make_interval(mins => $1::integer) AND s.created_at <= o.observed_at
)
SELECT o.observed_at, q.queued, q.running, q.oldest_queued_accepted_at, q.usable, q.running - q.usable,
       n.enabled, n.slots, r.accepted, r.system_error, r.recovery_exhausted
FROM observation o CROSS JOIN queue q CROSS JOIN nodes n CROSS JOIN recent r`

// ObserveJudgeOperations samples one MVCC snapshot under a bounded read-only
// transaction. database pins the destination by name; it is never logged.
func ObserveJudgeOperations(ctx context.Context, pool *pgxpool.Pool, database string, minutes int) (JudgeOperationsSnapshot, error) {
	zero := JudgeOperationsSnapshot{}
	if ctx == nil || pool == nil || database == "" || minutes < 1 || minutes > 60 {
		return zero, ErrJudgeSnapshotUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return zero, ErrJudgeSnapshotUnavailable
	}
	defer rollback(tx)
	var matches bool
	if err := tx.QueryRow(ctx, `SELECT current_database()::text = $1`, database).Scan(&matches); err != nil || !matches {
		return zero, ErrJudgeSnapshotUnavailable
	}
	out := JudgeOperationsSnapshot{Schema: "oj.judge-operations.v1", AcceptedWindowMinutes: minutes}
	if err := tx.QueryRow(ctx, judgeOperationsSQL, minutes).Scan(&out.ObservedAt, &out.Queued, &out.Running,
		&out.OldestQueuedAcceptedAt, &out.RunningWithCurrentLease, &out.RunningWithoutUsableLease, &out.ConfiguredEnabledNodes,
		&out.ConfiguredEnabledSlots, &out.RecentlyAccepted, &out.RecentWithSystemError, &out.RecentRecoveryExhausted); err != nil {
		return zero, ErrJudgeSnapshotUnavailable
	}
	out.ObservedAt = out.ObservedAt.UTC()
	if out.OldestQueuedAcceptedAt != nil {
		utc := out.OldestQueuedAcceptedAt.UTC()
		out.OldestQueuedAcceptedAt = &utc
	}
	if err := tx.Commit(ctx); err != nil || ctx.Err() != nil {
		return zero, ErrJudgeSnapshotUnavailable
	}
	return out, nil
}

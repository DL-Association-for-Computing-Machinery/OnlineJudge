package mailqueue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

// DeliveryResult is the only delivery information stored in River.
type DeliveryResult string

const (
	// DeliveryAccepted records a positive SMTP acknowledgement.
	DeliveryAccepted DeliveryResult = "accepted"
	// DeliveryRejected records a permanent failure.
	DeliveryRejected DeliveryResult = "rejected"
	// DeliveryObsolete records that the link can no longer be sent.
	DeliveryObsolete DeliveryResult = "obsolete"
	// DeliveryRetry records a temporary failure.
	DeliveryRetry DeliveryResult = "temporary_failure"
	// DeliveryUnknown records an unknown SMTP result.
	DeliveryUnknown DeliveryResult = "unknown"
	// DeliveryDeferred records that no send permit was obtained.
	DeliveryDeferred DeliveryResult = "deferred"
)

// Dispatcher uses the pinned River driver for claims but owns every mail
// result write. Do not run a River Client worker on this queue concurrently.
type Dispatcher struct {
	pool *pgxpool.Pool
	exec riverdriver.Executor
	id   string
}

// NewDispatcher checks migrations and creates a distinct claimant identity.
func NewDispatcher(ctx context.Context, pool *pgxpool.Pool) (*Dispatcher, error) {
	if ctx == nil || pool == nil {
		return nil, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	driver := riverpgxv5.New(pool)
	if err := validateMigrations(ctx, driver); err != nil {
		return nil, err
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	return &Dispatcher{pool: pool, exec: driver.GetExecutor(), id: "oj-mail-" + hex.EncodeToString(b[:])}, nil
}

// RescueOne returns one abandoned claim to the queue, or discards it if the
// attempt budget is exhausted. A crash counts as an attempt since SMTP may
// already have received the message.
func (d *Dispatcher) RescueOne(ctx context.Context) (bool, error) {
	if d == nil || d.pool == nil || ctx == nil {
		return false, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return false, errQueue
	}
	defer rollbackDispatch(tx)
	var id int64
	var attempt, ceiling int
	var payload string
	var canceled bool
	err = tx.QueryRow(ctx, `SELECT id, attempt, max_attempts, COALESCE(args->>'payload_id',''),
		metadata ? 'cancel_attempted_at' FROM river_job
		WHERE queue=$1 AND kind=$2 AND state='running' AND attempted_at < statement_timestamp()-interval '2 minutes'
		ORDER BY attempted_at, id LIMIT 1 FOR UPDATE SKIP LOCKED`, Queue, (Args{}).Kind()).
		Scan(&id, &attempt, &ceiling, &payload, &canceled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, errQueue
	}
	state := "available"
	if canceled {
		state = "cancelled"
	} else if attempt >= min(MaxAttempts, ceiling) || !identity.ValidID(payload) {
		state = "discarded"
	}
	if err := writeDispatchResult(ctx, tx, id, attempt, state, retryDelay(attempt), "abandoned"); err != nil {
		return false, err
	}
	if state != "available" && identity.ValidID(payload) {
		if err := identitypg.RetireEmailPayload(ctx, tx, payload); err != nil {
			return false, err
		}
	}
	if tx.Commit(ctx) != nil {
		return false, errQueue
	}
	return true, nil
}

// RunOne claims and processes at most one job. deliver must stop its external
// I/O when the context expires and must apply current send eligibility and
// shared admission. The caller owns the polling and shutdown cadence.
func (d *Dispatcher) RunOne(ctx context.Context, deliver func(context.Context, string) DeliveryResult) (bool, error) {
	if d == nil || d.pool == nil || ctx == nil || deliver == nil {
		return false, identity.ErrInvalid
	}
	send, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	call, cancel := context.WithTimeout(send, 5*time.Second)
	defer cancel()
	result, err := d.exec.JobGetAvailable(call, &riverdriver.JobGetAvailableParams{
		ClientID: d.id, Queue: Queue, Kind: []string{(Args{}).Kind()}, MaxToLock: 1, MaxAttemptedBy: 8,
	})
	if err != nil {
		return false, errQueue
	}
	if len(result.Jobs) == 0 {
		return false, nil
	}
	row := result.Jobs[0]
	if row == nil {
		return true, errQueue
	}
	job := &river.Job[Args]{JobRow: row}
	if result.DecodeErrors[row.ID] != nil || json.Unmarshal(row.EncodedArgs, &job.Args) != nil ||
		!identity.ValidID(job.Args.PayloadID) || row.Attempt < 1 || row.AttemptedAt == nil || len(row.AttemptedBy) == 0 {
		return true, errQueue
	}
	if err := d.checkClaim(call, job); err != nil {
		return true, err
	}
	outcome := DeliveryRejected
	if send.Err() != nil {
		outcome = DeliveryDeferred
	} else if row.Attempt <= MaxAttempts && row.Attempt <= row.MaxAttempts {
		outcome = deliver(send, job.Args.PayloadID)
	}
	return true, d.finish(context.WithoutCancel(ctx), job, outcome)
}

func (d *Dispatcher) checkClaim(ctx context.Context, job *river.Job[Args]) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return errQueue
	}
	defer rollbackDispatch(tx)
	return lockClaim(ctx, tx, job)
}

func (d *Dispatcher) finish(ctx context.Context, job *river.Job[Args], outcome DeliveryResult) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return errQueue
	}
	defer rollbackDispatch(tx)
	if err := lockClaim(ctx, tx, job); err != nil {
		return err
	}
	attempt, state, delay := job.Attempt, "completed", time.Duration(0)
	switch outcome {
	case DeliveryDeferred:
		attempt, state, delay = attempt-1, "available", 2*time.Second
	case DeliveryRetry, DeliveryUnknown:
		state = "discarded"
		if attempt < min(MaxAttempts, job.MaxAttempts) {
			state, delay = "available", retryDelay(attempt)
		}
	case DeliveryAccepted, DeliveryRejected, DeliveryObsolete:
	default:
		return identity.ErrInvalid
	}
	if err := writeDispatchResult(ctx, tx, job.ID, attempt, state, delay, outcome); err != nil {
		return err
	}
	if state != "available" {
		if err := identitypg.RetireEmailPayload(ctx, tx, job.Args.PayloadID); err != nil {
			return err
		}
	}
	if tx.Commit(ctx) != nil {
		return errQueue
	}
	return nil
}

func writeDispatchResult(ctx context.Context, tx pgx.Tx, id int64, attempt int, state string, delay time.Duration, outcome DeliveryResult) error {
	tag, err := tx.Exec(ctx, `UPDATE river_job SET state=$2::river_job_state, attempt=$3,
		scheduled_at=CASE WHEN $2='available' THEN clock_timestamp()+$4::bigint*interval '1 millisecond' ELSE scheduled_at END,
		finalized_at=CASE WHEN $2='available' THEN NULL ELSE clock_timestamp() END,
		metadata=metadata || jsonb_build_object('oj_delivery_result',$5::text)
		WHERE id=$1`, id, state, attempt, delay.Milliseconds(), string(outcome))
	if err != nil || tag.RowsAffected() != 1 {
		return errQueue
	}
	return nil
}

func rollbackDispatch(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func retryDelay(attempt int) time.Duration {
	var b [1]byte
	_, _ = rand.Read(b[:])
	return time.Second*(1<<max(0, min(attempt, 5))) + time.Duration(b[0])*10*time.Millisecond
}

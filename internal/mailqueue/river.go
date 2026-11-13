// Package mailqueue adapts River's PostgreSQL queue for account email.
package mailqueue

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// Queue is dedicated to account email.
const Queue = "account_email"

// MaxAttempts includes the initial send attempt.
const MaxAttempts = 6

var errQueue = errors.New("account mail queue unavailable")

// Args contains only the ID used to retrieve the protected business payload.
type Args struct {
	PayloadID string `json:"payload_id"`
}

// Kind identifies the mail job to River.
func (Args) Kind() string { return "account_email_delivery" }

// Enqueuer inserts mail jobs without starting a worker.
type Enqueuer struct {
	client *river.Client[pgx.Tx]
}

// NewEnqueuer checks the River migration state before enabling writes. The
// caller owns the pool and applies application schema and runtime checks.
func NewEnqueuer(ctx context.Context, pool *pgxpool.Pool) (*Enqueuer, error) {
	if ctx == nil || pool == nil {
		return nil, identity.ErrInvalid
	}
	driver := riverpgxv5.New(pool)
	if err := validateMigrations(ctx, driver); err != nil {
		return nil, err
	}
	client, err := river.NewClient(driver, &river.Config{Logger: quietLogger()})
	if err != nil {
		return nil, errQueue
	}
	return &Enqueuer{client: client}, nil
}

// Enqueue writes using the caller's issuance transaction; it does not commit,
// retain the transaction or use the pool to insert the job.
func (e *Enqueuer) Enqueue(ctx context.Context, tx pgx.Tx, payloadID string) error {
	if e == nil || e.client == nil || ctx == nil || tx == nil || !identity.ValidID(payloadID) {
		return identity.ErrInvalid
	}
	_, err := e.client.InsertTx(ctx, tx, Args{PayloadID: payloadID}, &river.InsertOpts{Queue: Queue, MaxAttempts: MaxAttempts})
	if err != nil {
		return errQueue
	}
	return nil
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

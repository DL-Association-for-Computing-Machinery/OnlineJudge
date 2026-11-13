package maildelivery

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailmessage"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailtransport"
)

// Worker is a serial mail sender using one authoritative PostgreSQL pool.
type Worker struct {
	queue   *mailqueue.Dispatcher
	attempt *Attempt
}

// NewWorker wires the shared permit, current payload reader and SMTP sender.
func NewWorker(ctx context.Context, pool *pgxpool.Pool, protector *emailtoken.Protector,
	renderer *emailmessage.Renderer, smtp *mailtransport.Sender,
) (*Worker, error) {
	if smtp == nil {
		return nil, errors.New("mail sender required")
	}
	queue, err := mailqueue.NewDispatcher(ctx, pool)
	if err != nil {
		return nil, err
	}
	rate, err := mailqueue.NewSendRate(ctx, pool)
	if err != nil {
		return nil, err
	}
	reader, err := identitypg.NewEmailDeliveryReader(pool, protector)
	if err != nil {
		return nil, err
	}
	attempt, err := New(reader, renderer, rate.TryTake, smtp.Send)
	if err != nil {
		return nil, err
	}
	return &Worker{queue: queue, attempt: attempt}, nil
}

// Run waits for cancellation or a storage error. Run only once per worker.
func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.queue == nil || ctx == nil {
		return errors.New("mail worker unavailable")
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if _, err := w.queue.RescueOne(ctx); err != nil && ctx.Err() == nil {
			return err
		}
		if _, err := w.queue.RunOne(ctx, w.deliver); err != nil && !errors.Is(err, mailqueue.ErrStaleClaim) && ctx.Err() == nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
	return nil
}

func (w *Worker) deliver(ctx context.Context, id string) mailqueue.DeliveryResult {
	switch w.attempt.Run(ctx, id) {
	case Accepted:
		return mailqueue.DeliveryAccepted
	case Rejected:
		return mailqueue.DeliveryRejected
	case Obsolete:
		return mailqueue.DeliveryObsolete
	case Unknown:
		return mailqueue.DeliveryUnknown
	case Deferred:
		return mailqueue.DeliveryDeferred
	default:
		return mailqueue.DeliveryRetry
	}
}

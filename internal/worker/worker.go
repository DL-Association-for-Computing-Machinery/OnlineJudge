// Package worker coordinates one exclusively owned execution slot. It does not
// create sandboxes, persist pending reports, or recover abandoned leases.
package worker

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
)

// Claimer returns a lease, an explicit empty queue, or an error. Implementations
// must bound requests and must not retry ambiguous claims.
type Claimer interface {
	Claim(context.Context) (submission.Lease, error)
}

// Fetcher returns a complete, verified material set under the supplied lease.
type Fetcher interface {
	Fetch(context.Context, submission.Lease) (materials.Descriptor, map[string][]byte, error)
}

// Executor owns lease supervision, sandbox cancellation, stop confirmation and
// cleanup. It must drain all work before returning and return no report on error.
// execution.LeasedRunner implements this contract.
type Executor interface {
	JudgeReport(context.Context, submission.Lease, materials.Descriptor, map[string][]byte) (submission.Result, error)
}

// Reporter acknowledges a frozen result or returns an error. Only
// workerclient.ErrReportUnknown permits a retry; requests must honor ctx.
type Reporter interface {
	Report(context.Context, string, submission.Result) error
}

// ErrStarted prevents both concurrent runs and reuse after an uncertain outcome.
var ErrStarted = errors.New("worker already started; create a verified new slot before recovery")

// Worker serializes acquisition, execution and reporting. Dependencies must be
// exclusively owned by this slot. Callers close transports only after Run exits.
type Worker struct {
	claim                 Claimer
	fetch                 Fetcher
	execute               Executor
	report                Reporter
	started               atomic.Bool
	idleDelay, retryDelay time.Duration
}

// New constructs a single-use worker. Empty queues wait one second; unknown
// report acknowledgements retry at most twice, one second apart. These bounds
// apply to the pinned M0 experiment, not a production recovery policy.
func New(claim Claimer, fetch Fetcher, execute Executor, report Reporter) (*Worker, error) {
	if claim == nil || fetch == nil || execute == nil || report == nil {
		return nil, errors.New("worker requires claim, materials, execution and reporting")
	}
	return &Worker{claim: claim, fetch: fetch, execute: execute, report: report,
		idleDelay: time.Second, retryDelay: time.Second}, nil
}

// Run polls until cancellation or the first error. It never claims another task
// until the current frozen report is acknowledged. Claim ambiguity, material or
// execution failure, terminal rejection, and exhausted retries all stop the slot.
// Cancellation is forwarded to the active operation; execution must confirm its
// stop and drain before returning. No detached reporting or shutdown retry occurs.
// Reports live only in memory: process death still requires controller recovery.
func (w *Worker) Run(ctx context.Context) error {
	if !w.started.CompareAndSwap(false, true) {
		return ErrStarted
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		lease, err := w.claim.Claim(ctx)
		if ctx.Err() != nil {
			return errors.Join(ctx.Err(), err)
		}
		if errors.Is(err, submission.ErrNoTask) {
			if err := wait(ctx, w.idleDelay); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("worker claim: %w", err)
		}
		if err := w.attempt(ctx, lease); err != nil {
			return err
		}
	}
}

func (w *Worker) attempt(ctx context.Context, lease submission.Lease) error {
	d, blobs, err := w.fetch.Fetch(ctx, lease)
	if err != nil || ctx.Err() != nil {
		return fmt.Errorf("worker materials: %w", errors.Join(err, ctx.Err()))
	}
	frozen, err := w.execute.JudgeReport(ctx, lease, d, blobs)
	if err != nil || ctx.Err() != nil {
		return fmt.Errorf("worker execution: %w", errors.Join(err, ctx.Err()))
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = w.report.Report(ctx, lease.Token, frozen)
		if ctx.Err() != nil {
			return errors.Join(ctx.Err(), err)
		}
		if err == nil {
			return nil
		}
		if !errors.Is(err, workerclient.ErrReportUnknown) || attempt == 2 {
			return fmt.Errorf("worker report: %w", err)
		}
		if err := wait(ctx, w.retryDelay); err != nil {
			return err
		}
	}
	panic("unreachable report loop")
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

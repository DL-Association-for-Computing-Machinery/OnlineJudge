package execution

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
)

var (
	// ErrLeaseLost means authority is absent or unknown; no result may be reported.
	ErrLeaseLost = errors.New("execution lease unavailable")
	// ErrStopUnconfirmed means the host could not prove that execution stopped.
	ErrStopUnconfirmed = errors.New("sandbox stop unconfirmed")
)

// RenewLease must use the authenticated controller's fixed 30-second renewal
// contract, return nil only on a validated acknowledgement, and honor ctx.
// Errors must not include credentials or response bodies.
type RenewLease func(ctx context.Context, token string) error

// StopSandbox must stop the entire exclusively owned sandbox, fence subsequent
// requests, and verify that its process tree is gone before returning nil.
// Cancelling HTTP or observing a request error is not sufficient confirmation.
// It must honor ctx and must not affect a shared executor.
type StopSandbox func(context.Context) error

// LeasedRunner owns one runner and sandbox exclusively, including stop/cleanup.
// An interrupted attempt permanently quarantines it, even after a confirmed stop.
// Recovery requires a fresh verified sandbox and a new LeasedRunner.
type LeasedRunner struct {
	runner                                        *Runner
	renew                                         RenewLease
	stop                                          StopSandbox
	busy                                          atomic.Bool
	quarantined                                   atomic.Bool
	budget, interval, requestTimeout, stopTimeout time.Duration
}

// NewLeased adds lease supervision to bounded execution. A preflight renewal
// avoids trusting a stale claim or comparing wall clocks across hosts. The local
// monotonic budget starts BEFORE the request, with five seconds of safety margin
// against the controller's fixed 30-second lease. It never retries unknown renewals.
func NewLeased(s Sandbox, renew RenewLease, stop StopSandbox) *LeasedRunner {
	return &LeasedRunner{runner: New(s), renew: renew, stop: stop,
		budget: 25 * time.Second, interval: 5 * time.Second,
		requestTimeout: 3 * time.Second, stopTimeout: 10 * time.Second}
}

// Judge renews before and during execution. Any loss of authority cancels work,
// confirms host-side termination using an independent bounded context, and drains
// execution/cleanup before releasing capacity. It never reports to the controller.
func (r *LeasedRunner) Judge(ctx context.Context, token string, d materials.Descriptor, blobs map[string][]byte) (Result, error) {
	failure := Result{Verdict: "SE"}
	if !r.busy.CompareAndSwap(false, true) {
		return failure, ErrBusy
	}
	defer r.busy.Store(false)
	if r.quarantined.Load() {
		return failure, errors.New("sandbox requires recovery")
	}
	if r.renew == nil || r.stop == nil {
		return failure, errors.New("lease renewal and stop confirmation required")
	}
	if err := ctx.Err(); err != nil {
		return failure, err
	}
	deadline, err := r.refresh(ctx, token, time.Now().Add(r.requestTimeout))
	if err != nil {
		return failure, errors.Join(ErrLeaseLost, err)
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type outcome struct {
		result Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() { result, err := r.runner.Judge(workCtx, d, blobs); done <- outcome{result, err} }()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	abort := func(cause error, completed *outcome) (Result, error) {
		r.quarantined.Store(true)
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), r.stopTimeout)
		stopErr := r.stop(stopCtx)
		if stopErr == nil {
			stopErr = stopCtx.Err()
		}
		stopCancel()
		if stopErr != nil {
			cause = errors.Join(cause, ErrStopUnconfirmed, stopErr)
		}
		// Sandbox implementations must honor context. Waiting also prevents a late
		// local request from escaping into a recovered executor after we return.
		if completed == nil {
			o := <-done
			completed = &o
		}
		completed.result.Verdict = "SE"
		return completed.result, errors.Join(cause, completed.err)
	}
	for {
		select {
		case <-ctx.Done():
			return abort(ctx.Err(), nil)
		case <-timer.C:
			return abort(ErrLeaseLost, nil)
		case <-ticker.C:
			next, err := r.refresh(ctx, token, deadline)
			if err != nil {
				return abort(errors.Join(ErrLeaseLost, err), nil)
			}
			deadline = next
			timer.Reset(time.Until(deadline))
		case o := <-done:
			if ctx.Err() != nil {
				return abort(ctx.Err(), &o)
			}
			if !time.Now().Before(deadline) {
				return abort(ErrLeaseLost, &o)
			}
			if o.err != nil {
				return abort(o.err, &o)
			}
			return o.result, nil
		}
	}
}

func (r *LeasedRunner) refresh(ctx context.Context, token string, previous time.Time) (time.Time, error) {
	start := time.Now()
	deadline := start.Add(r.requestTimeout)
	if previous.Before(deadline) {
		deadline = previous
	}
	if !start.Before(deadline) {
		return time.Time{}, ErrLeaseLost
	}
	requestCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	err := r.renew(requestCtx, token)
	if err == nil {
		err = requestCtx.Err()
	}
	next := start.Add(r.budget)
	if err == nil && (!time.Now().Before(deadline) || !time.Now().Before(next)) {
		err = ErrLeaseLost
	}
	return next, err
}

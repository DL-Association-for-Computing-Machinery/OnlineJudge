package execution

import (
	"context"
	"errors"
	"time"
)

// ReferenceLeaseOutcome keeps authority/work failure distinct from stop proof.
// WorkStarted says the callback was launched, not that a sandbox was started.
type ReferenceLeaseOutcome struct {
	WorkStarted bool
	WorkErr     error
	StopErr     error
}

// RunReferenceLease supervises one complete reference operation, including
// startup. The owner must hold the host and target slot locks throughout. Work
// must honor its context; Stop must handle concurrent work/startup and fence
// late dispatch. A successful initial renewal is required before Work runs.
// Stop is called on every outcome after Work starts, including success.
func RunReferenceLease(ctx context.Context, renew func(context.Context) error, work func(context.Context) error, stop StopSandbox) ReferenceLeaseOutcome {
	return runReferenceLease(ctx, renew, work, stop, referenceLeaseTiming{
		budget: 25 * time.Second, interval: 5 * time.Second, request: 3 * time.Second, stop: 15 * time.Second,
	})
}

type referenceLeaseTiming struct{ budget, interval, request, stop time.Duration }

func runReferenceLease(ctx context.Context, renew, work func(context.Context) error, stop StopSandbox, timing referenceLeaseTiming) ReferenceLeaseOutcome {
	if ctx == nil || renew == nil || work == nil || stop == nil {
		return ReferenceLeaseOutcome{WorkErr: ErrLeaseLost}
	}
	deadline, err := refreshLease(ctx, renew, time.Now().Add(timing.request), timing.budget, timing.request)
	if err != nil {
		return ReferenceLeaseOutcome{WorkErr: errors.Join(ErrLeaseLost, err)}
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- work(workCtx) }()
	ticker := time.NewTicker(timing.interval)
	defer ticker.Stop()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	finish := func(cause error, workDone bool) ReferenceLeaseOutcome {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), timing.stop)
		stopErr := stop(stopCtx)
		if stopErr == nil {
			stopErr = stopCtx.Err()
		}
		stopCancel()
		if stopErr != nil {
			stopErr = errors.Join(ErrStopUnconfirmed, stopErr)
		}
		if !workDone {
			cause = errors.Join(cause, <-done)
		}
		if cause == nil {
			cause = ctx.Err()
			if cause == nil && !time.Now().Before(deadline) {
				cause = ErrLeaseLost
			}
		}
		return ReferenceLeaseOutcome{WorkStarted: true, WorkErr: cause, StopErr: stopErr}
	}
	for {
		select {
		case <-ctx.Done():
			return finish(ctx.Err(), false)
		case <-timer.C:
			return finish(ErrLeaseLost, false)
		case <-ticker.C:
			next, err := refreshLease(ctx, renew, deadline, timing.budget, timing.request)
			if err != nil {
				return finish(errors.Join(ErrLeaseLost, err), false)
			}
			deadline = next
			timer.Reset(time.Until(deadline))
		case err := <-done:
			return finish(err, true)
		}
	}
}

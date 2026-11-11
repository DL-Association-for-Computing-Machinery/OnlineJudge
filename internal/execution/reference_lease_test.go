package execution

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReferenceLeasePreflightRefusalNeverStartsWork(t *testing.T) {
	unknown := errors.New("renewal acknowledgement unknown")
	out := RunReferenceLease(t.Context(), func(context.Context) error { return unknown }, func(context.Context) error {
		t.Error("work launched without an acknowledged renewal")
		return nil
	}, func(context.Context) error {
		t.Error("stop invoked without starting work")
		return nil
	})
	if out.WorkStarted || !errors.Is(out.WorkErr, ErrLeaseLost) || !errors.Is(out.WorkErr, unknown) || out.StopErr != nil {
		t.Fatalf("unexpected refusal: %+v", out)
	}
}

func TestReferenceLeaseRenewalLossCancelsStopsAndDrains(t *testing.T) {
	timing := referenceLeaseTiming{budget: time.Second, interval: 5 * time.Millisecond, request: time.Second, stop: time.Second}
	started, cancelled, stopped, drained := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	unknown := errors.New("renewal acknowledgement unknown")
	stopFailure := errors.New("stop not observed")
	renewals := 0
	out := runReferenceLease(t.Context(), func(ctx context.Context) error {
		renewals++
		if renewals == 1 {
			return nil
		}
		select {
		case <-started:
			return unknown
		case <-ctx.Done():
			return ctx.Err()
		}
	}, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-stopped // Simulate an in-flight adapter call released by host fencing.
		close(drained)
		return ctx.Err()
	}, func(ctx context.Context) error {
		defer close(stopped)
		select {
		case <-cancelled:
			return stopFailure
		case <-ctx.Done():
			return ctx.Err()
		}
	}, timing)
	if !out.WorkStarted || !errors.Is(out.WorkErr, ErrLeaseLost) || !errors.Is(out.WorkErr, unknown) ||
		!errors.Is(out.StopErr, ErrStopUnconfirmed) || !errors.Is(out.StopErr, stopFailure) || renewals != 2 {
		t.Fatalf("renewal loss was not fenced: %+v; renewals=%d", out, renewals)
	}
	select {
	case <-drained:
	default:
		t.Fatal("supervisor returned before draining work")
	}
}

func TestReferenceLeaseSuccessStillRequiresStopProof(t *testing.T) {
	for _, late := range []bool{false, true} {
		name := "confirmed"
		if late {
			name = "late acknowledgement"
		}
		t.Run(name, func(t *testing.T) {
			timing := referenceLeaseTiming{budget: time.Second, interval: time.Hour, request: time.Second, stop: time.Second}
			if late {
				timing.stop = 10 * time.Millisecond
			}
			stops := 0
			out := runReferenceLease(t.Context(), func(context.Context) error { return nil }, func(context.Context) error { return nil }, func(ctx context.Context) error {
				stops++
				if late {
					<-ctx.Done()
				}
				return nil
			}, timing)
			if !out.WorkStarted || out.WorkErr != nil || stops != 1 || errors.Is(out.StopErr, ErrStopUnconfirmed) != late || (out.StopErr != nil) != late {
				t.Fatalf("incorrect stop evidence: %+v; stops=%d", out, stops)
			}
		})
	}
}

func TestReferenceLeaseRejectsLateRenewalAcknowledgement(t *testing.T) {
	timing := referenceLeaseTiming{budget: time.Second, interval: 5 * time.Millisecond, request: 30 * time.Millisecond, stop: time.Second}
	calls := 0
	out := runReferenceLease(t.Context(), func(ctx context.Context) error {
		calls++
		if calls > 1 {
			<-ctx.Done()
		}
		return nil // A nominal success delivered after its deadline is not usable.
	}, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}, func(ctx context.Context) error { return ctx.Err() }, timing)
	if !out.WorkStarted || !errors.Is(out.WorkErr, ErrLeaseLost) || out.StopErr != nil || calls != 2 {
		t.Fatalf("late acknowledgement was not fenced: %+v calls=%d", out, calls)
	}
}

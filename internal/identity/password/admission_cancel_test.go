package password

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdmissionWaitingCancellationKeepsOwnership(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
			h := testHasher(t)
			var calls atomic.Int32
			derive := h.derive
			h.derive = func(password, salt []byte, p Parameters) []byte {
				calls.Add(1)
				return derive(password, salt, p)
			}
			b := boundedForTest(t, h, AdmissionLimits{MaxInFlight: 1, MaxWaiting: 1, MaxQueueWait: time.Second})
			if err := b.slots.Acquire(context.Background(), 1); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
			}
			defer cancel()
			queued := startAdmission(b, true, ctx, vectorRecord(t))
			want := context.Canceled
			if deadline {
				want = context.DeadlineExceeded
			} else {
				awaitWaiting(t, b, 1)
				cancel()
			}
			assertAdmissionError(t, awaitAdmission(t, queued), want)
			if calls.Load() != 0 || len(b.waiting) != 0 || b.slots.TryAcquire(1) {
				t.Fatal("canceled waiter derived, changed a holder's slot, or leaked waiting capacity")
			}
			// A replacement waiter can take the same queue capacity and succeed.
			replacement := startAdmission(b, false, context.Background(), Record{})
			awaitWaiting(t, b, 1)
			b.slots.Release(1)
			if got := awaitAdmission(t, replacement); got.err != nil {
				t.Fatal(got.err)
			}
		})
	}
}

func TestAdmissionCanceledWaiterRacesPermitRelease(t *testing.T) {
	h := testHasher(t)
	var calls atomic.Int32
	h.derive = func(_ []byte, _ []byte, p Parameters) []byte {
		calls.Add(1)
		return make([]byte, p.DigestBytes)
	}
	b := boundedForTest(t, h, AdmissionLimits{MaxInFlight: 1, MaxWaiting: 1, MaxQueueWait: time.Second})
	if err := b.slots.Acquire(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queued := startAdmission(b, false, ctx, Record{})
	awaitWaiting(t, b, 1)
	// Cancellation is observable before the permit becomes available, without
	// waiting for the blocked goroutine to react. Both paths must leave it free.
	cancel()
	b.slots.Release(1)
	assertAdmissionError(t, awaitAdmission(t, queued), context.Canceled)
	if calls.Load() != 0 || len(b.waiting) != 0 || !b.slots.TryAcquire(1) {
		t.Fatal("cancellation/permit race started work or leaked capacity")
	}
	b.slots.Release(1)
}

func TestAdmissionRunningCancellationRetainsSlotAndDiscardsResult(t *testing.T) {
	for _, verify := range []bool{false, true} {
		t.Run(map[bool]string{false: "Create", true: "Verify"}[verify], func(t *testing.T) {
			h := testHasher(t)
			entered, release := make(chan struct{}, 1), make(chan struct{})
			h.derive = func(_ []byte, _ []byte, p Parameters) []byte {
				entered <- struct{}{}
				<-release
				return make([]byte, p.DigestBytes)
			}
			t.Cleanup(func() { close(release) })
			b := boundedForTest(t, h, AdmissionLimits{MaxInFlight: 1})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := vectorRecord(t)
			r.Digest = make([]byte, len(r.Digest))
			running := startAdmission(b, verify, ctx, r)
			awaitAdmissionSignal(t, entered)
			cancel()
			select {
			case <-running:
				t.Fatal("returned before actual calculation ended")
			default:
			}
			for _, other := range []bool{false, true} {
				assertAdmissionError(t, admissionCall(b, other, context.Background(), r), ErrBusy)
			}
			release <- struct{}{}
			assertAdmissionError(t, awaitAdmission(t, running), context.Canceled)
			if !b.slots.TryAcquire(1) {
				t.Fatal("finished canceled calculation leaked slot")
			}
			b.slots.Release(1)
		})
	}
}

func TestAdmissionQueueTimerDoesNotLimitRunningCalculation(t *testing.T) {
	h := testHasher(t)
	entered, release := make(chan struct{}, 1), make(chan struct{})
	h.derive = func(_ []byte, _ []byte, p Parameters) []byte {
		entered <- struct{}{}
		<-release
		return make([]byte, p.DigestBytes)
	}
	t.Cleanup(func() { close(release) })
	b := boundedForTest(t, h, AdmissionLimits{MaxInFlight: 1, MaxWaiting: 1, MaxQueueWait: time.Second})
	if err := b.slots.Acquire(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	queued := startAdmission(b, false, context.Background(), Record{})
	awaitWaiting(t, b, 1)
	b.slots.Release(1)
	awaitAdmissionSignal(t, entered)
	awaitWaiting(t, b, 0)
	// The queued call is now running. Cross its original queue budget while it
	// still owns the slot; a queue context must not become the runtime context.
	<-time.After(1100 * time.Millisecond)
	release <- struct{}{}
	if got := awaitAdmission(t, queued); got.err != nil || len(got.record.Digest) == 0 {
		t.Fatalf("queue timer affected running calculation: %v", got.err)
	}
}

func TestAdmissionCallerCancellationTakesPrecedenceOverCoreFailure(t *testing.T) {
	h := testHasher(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.salt = func([]byte) error { cancel(); return errors.New("synthetic random failure") }
	b := boundedForTest(t, h, AdmissionLimits{MaxInFlight: 1})
	assertAdmissionError(t, admissionCall(b, false, ctx, Record{}), context.Canceled)
	if !b.slots.TryAcquire(1) {
		t.Fatal("simultaneous cancellation/core error leaked slot")
	}
	b.slots.Release(1)
}

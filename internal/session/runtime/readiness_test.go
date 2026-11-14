package runtime

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadinessCoalescesAndFailsClosedDuringDrain(t *testing.T) {
	var checks atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	probe := &readinessProbe{check: func(context.Context) bool {
		checks.Add(1)
		close(entered)
		<-release
		return true
	}}
	const count = 12
	var wait sync.WaitGroup
	for range count {
		wait.Go(func() {
			if !probe.ready(t.Context()) {
				t.Error("coalesced healthy observation was lost")
			}
		})
	}
	<-entered
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if probe.ready(canceled) {
		t.Fatal("canceled check reported ready")
	}
	close(release)
	wait.Wait()
	if !probe.ready(t.Context()) || checks.Load() != 1 {
		t.Fatal("probe did not reuse the same observation")
	}
	r := newRuntime(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), func() error { return nil })
	r.probe = probe
	if !r.Ready(t.Context()) {
		t.Fatal("running probe is unavailable")
	}
	if err := r.Close(t.Context()); err != nil || r.Ready(t.Context()) {
		t.Fatal("drained runtime still reported its cached ready observation")
	}
}

func TestCanceledProbeLeaderDoesNotCacheDependencyFailure(t *testing.T) {
	var checks int
	ctx, cancel := context.WithCancel(t.Context())
	p := &readinessProbe{check: func(context.Context) bool {
		checks++
		if checks == 1 {
			cancel()
			return false
		}
		return true
	}}
	if p.ready(ctx) || p.ready(t.Context()) || checks != 1 || !p.until.IsZero() {
		t.Fatal("a canceled requester cached an outage or bypassed start throttling")
	}
	p.next = time.Time{}
	if !p.ready(t.Context()) || checks != 2 {
		t.Fatal("a canceled requester poisoned the next observation")
	}
}

func TestCloseWaitsForInFlightReadinessAndSuppressesItsSuccess(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(release) }) }
	defer finish()
	var closed atomic.Bool
	r := newRuntime(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), func() error {
		closed.Store(true)
		return nil
	})
	r.probe = &readinessProbe{check: func(context.Context) bool {
		close(entered)
		<-release
		return true
	}}
	result := make(chan bool, 1)
	go func() { result <- r.Ready(t.Context()) }()
	<-entered
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := r.Close(ctx); err != context.Canceled || closed.Load() {
		t.Fatal("close did not wait for the admitted probe")
	}
	finish()
	if <-result {
		t.Fatal("the in-flight probe reported ready after draining started")
	}
	if err := r.Close(t.Context()); err != nil || !closed.Load() {
		t.Fatal("cleanup did not finish after the probe returned")
	}
}

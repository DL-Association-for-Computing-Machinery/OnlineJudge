package execution

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

type waitingSandbox struct {
	*fakeSandbox
	started chan struct{}
	release chan struct{}
	compile bool
}

func (s *waitingSandbox) wait(ctx context.Context) error {
	close(s.started)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.release:
		return nil
	}
}
func (s *waitingSandbox) Compile(ctx context.Context, p experiment.ExecutionProfile, source string) (Observation, error) {
	if s.compile {
		if err := s.wait(ctx); err != nil {
			return Observation{}, err
		}
	}
	return s.fakeSandbox.Compile(ctx, p, source)
}
func (s *waitingSandbox) Run(ctx context.Context, p experiment.ExecutionProfile, id string, input []byte) (Observation, error) {
	if !s.compile && len(s.runs) == 2 {
		if err := s.wait(ctx); err != nil {
			return Observation{}, err
		}
	}
	return s.fakeSandbox.Run(ctx, p, id, input)
}
func fastLeased(s Sandbox, renew RenewLease, stop StopSandbox) *LeasedRunner {
	r := NewLeased(s, renew, stop)
	r.interval, r.budget, r.requestTimeout, r.stopTimeout = 10*time.Millisecond, time.Second, 100*time.Millisecond, time.Second
	return r
}

func TestLeaseFailureStopsExecution(t *testing.T) {
	for _, phase := range []string{"compile", "run"} {
		for _, failure := range []string{"revoked", "expired", "transport", "timeout", "late success"} {
			t.Run(phase+"/"+failure, func(t *testing.T) {
				d, b := fixture()
				s := &waitingSandbox{fakeSandbox: goodSandbox(), started: make(chan struct{}), release: make(chan struct{}), compile: phase == "compile"}
				var renewals, stops atomic.Int32
				r := fastLeased(s, func(ctx context.Context, token string) error {
					if token != "test-token" {
						t.Error("token changed")
					}
					if renewals.Add(1) == 1 {
						return nil
					}
					select {
					case <-s.started:
					case <-ctx.Done():
						return ctx.Err()
					}
					switch failure {
					case "revoked":
						return submission.ErrNode
					case "expired":
						return submission.ErrLease
					case "timeout":
						<-ctx.Done()
						return ctx.Err()
					case "late success":
						<-ctx.Done()
						return nil
					default:
						return errors.New("transport unavailable")
					}
				}, func(ctx context.Context) error {
					stops.Add(1)
					if ctx.Err() != nil {
						t.Error("stop inherited cancelled context")
					}
					return nil
				})
				result, err := r.Judge(context.Background(), "test-token", d, b)
				if !errors.Is(err, ErrLeaseLost) || result.Verdict != "SE" || stops.Load() != 1 || renewals.Load() != 2 {
					t.Fatalf("result=%+v err=%v stops=%d renewals=%d", result, err, stops.Load(), renewals.Load())
				}
				if result.Points[1].Verdict != "NOT_RUN" || result.Points[1].Usage != nil {
					t.Fatal("later test executed")
				}
				before := len(s.calls)
				if _, err := r.Judge(context.Background(), "test-token", d, b); err == nil || len(s.calls) != before || renewals.Load() != 2 {
					t.Fatal("quarantined slot reused")
				}
			})
		}
	}
}

func TestLeasePreflightAndLocalExpiry(t *testing.T) {
	d, b := fixture()
	s := goodSandbox()
	r := fastLeased(s, func(context.Context, string) error { return submission.ErrLease }, func(context.Context) error { t.Fatal("nothing started"); return nil })
	if _, err := r.Judge(context.Background(), "token", d, b); !errors.Is(err, ErrLeaseLost) || len(s.calls) != 0 {
		t.Fatal("preflight failed open")
	}
	waiting := &waitingSandbox{fakeSandbox: goodSandbox(), started: make(chan struct{}), release: make(chan struct{})}
	stops := 0
	r = fastLeased(waiting, func(context.Context, string) error { return nil }, func(context.Context) error { stops++; return nil })
	r.budget, r.interval = 30*time.Millisecond, time.Second
	if _, err := r.Judge(context.Background(), "token", d, b); !errors.Is(err, ErrLeaseLost) || stops != 1 {
		t.Fatalf("expiry: %v stops=%d", err, stops)
	}
}

func TestLeaseRenewalKeepsAttemptAlive(t *testing.T) {
	d, b := fixture()
	s := &waitingSandbox{fakeSandbox: goodSandbox(), started: make(chan struct{}), release: make(chan struct{})}
	renewals := 0
	r := fastLeased(s, func(context.Context, string) error {
		renewals++
		if renewals == 12 {
			close(s.release)
		}
		return nil
	}, func(context.Context) error { t.Error("healthy attempt stopped"); return nil })
	r.budget, r.interval = 250*time.Millisecond, 30*time.Millisecond
	result, err := r.Judge(context.Background(), "token", d, b)
	if err != nil || result.Verdict != "AC" || renewals < 12 {
		t.Fatalf("%+v %v renewals=%d", result, err, renewals)
	}
}

func TestCancellationRetainsSlotUntilStopConfirmed(t *testing.T) {
	for _, stopFails := range []bool{false, true} {
		d, b := fixture()
		s := &waitingSandbox{fakeSandbox: goodSandbox(), started: make(chan struct{}), release: make(chan struct{})}
		ctx, cancel := context.WithCancel(context.Background())
		stopping, releaseStop := make(chan struct{}), make(chan struct{})
		r := fastLeased(s, func(context.Context, string) error { return nil }, func(ctx context.Context) error {
			close(stopping)
			<-releaseStop
			if ctx.Err() != nil {
				t.Error("stop context cancelled")
			}
			if stopFails {
				return errors.New("host unavailable")
			}
			return nil
		})
		done := make(chan error, 1)
		go func() { _, err := r.Judge(ctx, "token", d, b); done <- err }()
		<-s.started
		cancel()
		<-stopping
		if _, err := r.Judge(context.Background(), "token", d, b); !errors.Is(err, ErrBusy) {
			t.Fatalf("slot released during stop: %v", err)
		}
		select {
		case err := <-done:
			t.Fatalf("returned before stop confirmation: %v", err)
		default:
		}
		close(releaseStop)
		err := <-done
		if !errors.Is(err, context.Canceled) || errors.Is(err, ErrStopUnconfirmed) != stopFails {
			t.Fatalf("stop outcome: %v", err)
		}
	}
}

func TestCompletionDoesNotOverrideFailedRenewal(t *testing.T) {
	d, b := fixture()
	s := &waitingSandbox{fakeSandbox: goodSandbox(), started: make(chan struct{}), release: make(chan struct{})}
	calls, stops := 0, 0
	r := fastLeased(s, func(context.Context, string) error {
		calls++
		if calls == 1 {
			return nil
		}
		close(s.release)
		return submission.ErrNode
	}, func(context.Context) error { stops++; return nil })
	result, err := r.Judge(context.Background(), "token", d, b)
	if !errors.Is(err, ErrLeaseLost) || result.Verdict != "SE" || stops != 1 {
		t.Fatalf("%+v %v", result, err)
	}
}

package worker

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
)

func TestDrainBeforeClaimAndSharedSingleUse(t *testing.T) {
	w, calls := fixture(t)
	drain := make(chan struct{})
	close(drain)
	if err := w.RunWithDrain(context.Background(), drain); err != nil || len(*calls) != 0 {
		t.Fatalf("closed drain claimed work: %v %v", err, *calls)
	}
	if err := w.Run(context.Background()); !errors.Is(err, ErrStarted) {
		t.Fatalf("Run reused a drained worker: %v", err)
	}
	if err := w.RunWithDrain(context.Background(), drain); !errors.Is(err, ErrStarted) {
		t.Fatalf("RunWithDrain reused a drained worker: %v", err)
	}
}

func TestDrainCompletesAdmittedAttempt(t *testing.T) {
	for _, stage := range []string{"claim", "fetch", "execute", "report", "retry"} {
		t.Run(stage, func(t *testing.T) {
			w, calls := fixture(t)
			drain := make(chan struct{})
			request := sync.OnceFunc(func() { close(drain) })
			claim, fetch, execute, report := w.claim, w.fetch, w.execute, w.report
			w.claim = claimFunc(func(ctx context.Context) (submission.Lease, error) {
				if stage == "claim" {
					request()
				}
				return claim.Claim(ctx)
			})
			w.fetch = fetchFunc(func(ctx context.Context, l submission.Lease) (materials.Descriptor, map[string][]byte, error) {
				if stage == "fetch" {
					request()
				}
				return fetch.Fetch(ctx, l)
			})
			w.execute = executeFunc(func(ctx context.Context, l submission.Lease, d materials.Descriptor, b map[string][]byte) (submission.Result, error) {
				if stage == "execute" {
					request()
				}
				if ctx.Err() != nil {
					t.Error("drain canceled execution")
				}
				return execute.JudgeReport(ctx, l, d, b)
			})
			reports := 0
			w.report = reportFunc(func(ctx context.Context, token string, r submission.Result) error {
				if stage == "report" || stage == "retry" {
					request()
				}
				if ctx.Err() != nil {
					t.Error("drain canceled reporting")
				}
				if err := report.Report(ctx, token, r); err != nil {
					return err
				}
				reports++
				if stage == "retry" && reports < 3 {
					return workerclient.ErrReportUnknown
				}
				return nil
			})
			if err := w.RunWithDrain(context.Background(), drain); err != nil {
				t.Fatal(err)
			}
			want := []string{"claim", "fetch", "execute", "report"}
			if stage == "retry" {
				want = append(want, "report", "report")
			}
			if !reflect.DeepEqual(*calls, want) {
				t.Fatalf("drain sequence=%v want=%v", *calls, want)
			}
		})
	}
}

func TestDrainWakesIdleWorker(t *testing.T) {
	w, calls := fixture(t)
	w.idleDelay = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	drain, claimed := make(chan struct{}), make(chan struct{}, 1)
	w.claim = claimFunc(func(context.Context) (submission.Lease, error) {
		claimed <- struct{}{}
		return submission.Lease{}, submission.ErrNoTask
	})
	done := make(chan error, 1)
	go func() { done <- w.RunWithDrain(ctx, drain) }()
	select {
	case <-claimed:
	case <-ctx.Done():
		t.Fatal("idle claim not reached")
	}
	if err := w.Run(ctx); !errors.Is(err, ErrStarted) {
		t.Fatalf("concurrent Run: %v", err)
	}
	close(drain)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("drain did not interrupt idle waiting")
	}
	if len(*calls) != 0 {
		t.Fatal("idle drain called downstream operations")
	}
}

func TestDrainNeverMasksOperationFailure(t *testing.T) {
	for _, stage := range []string{"claim", "fetch", "execute", "report"} {
		t.Run(stage, func(t *testing.T) {
			w, calls := fixture(t)
			drain := make(chan struct{})
			request := sync.OnceFunc(func() { close(drain) })
			cause := workerclient.ErrClaimUnknown
			switch stage {
			case "claim":
				w.claim = claimFunc(func(context.Context) (submission.Lease, error) { request(); return submission.Lease{}, cause })
			case "fetch":
				cause = workerclient.ErrMaterials
				w.fetch = fetchFunc(func(context.Context, submission.Lease) (materials.Descriptor, map[string][]byte, error) {
					request()
					return materials.Descriptor{}, nil, cause
				})
			case "execute":
				cause = execution.ErrStopUnconfirmed
				w.execute = executeFunc(func(context.Context, submission.Lease, materials.Descriptor, map[string][]byte) (submission.Result, error) {
					request()
					return submission.Result{}, cause
				})
			case "report":
				cause = workerclient.ErrReportUnknown
				w.report = reportFunc(func(context.Context, string, submission.Result) error {
					*calls = append(*calls, "report")
					request()
					return cause
				})
			}
			if err := w.RunWithDrain(context.Background(), drain); !errors.Is(err, cause) {
				t.Fatalf("drain masked failure: %v", err)
			}
			if stage == "report" && !reflect.DeepEqual(*calls, []string{"claim", "fetch", "execute", "report", "report", "report"}) {
				t.Fatalf("drain changed unknown report retry bound: %v", *calls)
			}
		})
	}
}

func TestCancellationTakesPrecedenceOverDrain(t *testing.T) {
	for _, stage := range []string{"before", "empty-claim", "execution"} {
		t.Run(stage, func(t *testing.T) {
			w, _ := fixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			drain := make(chan struct{})
			switch stage {
			case "before":
				close(drain)
				cancel()
			case "empty-claim":
				w.claim = claimFunc(func(context.Context) (submission.Lease, error) {
					close(drain)
					cancel()
					return submission.Lease{}, submission.ErrNoTask
				})
			case "execution":
				w.execute = executeFunc(func(context.Context, submission.Lease, materials.Descriptor, map[string][]byte) (submission.Result, error) {
					close(drain)
					cancel()
					return submission.Result{}, execution.ErrStopUnconfirmed
				})
			}
			err := w.RunWithDrain(ctx, drain)
			if !errors.Is(err, context.Canceled) || stage == "execution" && !errors.Is(err, execution.ErrStopUnconfirmed) {
				t.Fatalf("cancellation/stop evidence lost: %v", err)
			}
		})
	}
}

func TestDrainPreservesTerminalReportRejection(t *testing.T) {
	for _, cause := range []error{submission.ErrLease, submission.ErrResultConflict} {
		t.Run(cause.Error(), func(t *testing.T) {
			w, calls := fixture(t)
			drain := make(chan struct{})
			request := sync.OnceFunc(func() { close(drain) })
			report := w.report
			w.report = reportFunc(func(ctx context.Context, token string, result submission.Result) error {
				if err := report.Report(ctx, token, result); err != nil {
					return err
				}
				request()
				return cause
			})
			if err := w.RunWithDrain(context.Background(), drain); !errors.Is(err, cause) {
				t.Fatalf("drain hid terminal report rejection: %v", err)
			}
			if !reflect.DeepEqual(*calls, []string{"claim", "fetch", "execute", "report"}) {
				t.Fatalf("terminal rejection retried or claimed again: %v", *calls)
			}
		})
	}
}

func TestIdleCancellationAndDrainBothReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	drain := make(chan struct{})
	close(drain)
	cancel()
	// Either ready select arm must retain cancellation, independent of which
	// event the scheduler chooses. Repetition exercises that selection.
	for range 64 {
		if _, err := waitIdle(ctx, drain, time.Hour); !errors.Is(err, context.Canceled) {
			t.Fatalf("idle drain hid ready cancellation: %v", err)
		}
	}
}

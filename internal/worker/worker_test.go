package worker

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
)

type claimFunc func(context.Context) (submission.Lease, error)

func (f claimFunc) Claim(ctx context.Context) (submission.Lease, error) { return f(ctx) }

type fetchFunc func(context.Context, submission.Lease) (materials.Descriptor, map[string][]byte, error)

func (f fetchFunc) Fetch(ctx context.Context, l submission.Lease) (materials.Descriptor, map[string][]byte, error) {
	return f(ctx, l)
}

type executeFunc func(context.Context, submission.Lease, materials.Descriptor, map[string][]byte) (submission.Result, error)

func (f executeFunc) JudgeReport(ctx context.Context, l submission.Lease, d materials.Descriptor, b map[string][]byte) (submission.Result, error) {
	return f(ctx, l, d, b)
}

type reportFunc func(context.Context, string, submission.Result) error

func (f reportFunc) Report(ctx context.Context, token string, r submission.Result) error {
	return f(ctx, token, r)
}

var testLease = submission.Lease{Token: strings.Repeat("a", 64), SubmissionID: strings.Repeat("b", 32), JudgeGeneration: 1, RecoveryGeneration: 1, AttemptNumber: 1}
var testResult = submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("c", 64)}

func fixture(t *testing.T) (*Worker, *[]string) {
	t.Helper()
	var calls []string
	w, err := New(claimFunc(func(context.Context) (submission.Lease, error) {
		calls = append(calls, "claim")
		return testLease, nil
	}), fetchFunc(func(_ context.Context, l submission.Lease) (materials.Descriptor, map[string][]byte, error) {
		calls = append(calls, "fetch")
		if l != testLease {
			t.Error("lease changed during fetch")
		}
		return materials.Descriptor{TaskMaterials: submission.TaskMaterials{SubmissionID: l.SubmissionID}}, map[string][]byte{"input": []byte("fixture")}, nil
	}), executeFunc(func(_ context.Context, l submission.Lease, d materials.Descriptor, b map[string][]byte) (submission.Result, error) {
		calls = append(calls, "execute")
		if l != testLease || d.SubmissionID != l.SubmissionID || string(b["input"]) != "fixture" {
			t.Error("execution inputs changed")
		}
		return testResult, nil
	}), reportFunc(func(_ context.Context, token string, r submission.Result) error {
		calls = append(calls, "report")
		if token != testLease.Token || r != testResult {
			t.Error("frozen report changed")
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	w.idleDelay, w.retryDelay = time.Millisecond, time.Millisecond
	return w, &calls
}

func TestSerialAttemptsAndFrozenRetry(t *testing.T) {
	w, calls := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	base := w.report
	reports := 0
	w.report = reportFunc(func(ctx context.Context, token string, r submission.Result) error {
		if err := base.Report(ctx, token, r); err != nil {
			return err
		}
		reports++
		if reports == 1 || reports == 2 {
			return workerclient.ErrReportUnknown
		}
		if reports == 4 {
			cancel()
		}
		return nil
	})
	if err := w.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	want := []string{"claim", "fetch", "execute", "report", "report", "report", "claim", "fetch", "execute", "report"}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("sequence=%v", *calls)
	}
	if err := w.Run(context.Background()); !errors.Is(err, ErrStarted) {
		t.Fatalf("reused slot: %v", err)
	}
}

func TestFailureStopsSlot(t *testing.T) {
	for _, tc := range []struct {
		stage   string
		cause   error
		reports int
	}{
		{"claim", workerclient.ErrClaimUnknown, 0},
		{"claim", submission.ErrNode, 0},
		{"fetch", workerclient.ErrMaterials, 0},
		{"fetch", submission.ErrLease, 0},
		{"execute", execution.ErrLeaseLost, 0},
		{"execute", execution.ErrStopUnconfirmed, 0},
		{"report", submission.ErrNode, 1},
		{"report", submission.ErrLease, 1},
		{"report", submission.ErrResultConflict, 1},
		{"report", submission.ErrInvalid, 1},
		{"report", workerclient.ErrReportUnknown, 3},
	} {
		t.Run(tc.stage+"/"+tc.cause.Error(), func(t *testing.T) {
			w, calls := fixture(t)
			switch tc.stage {
			case "claim":
				w.claim = claimFunc(func(context.Context) (submission.Lease, error) {
					*calls = append(*calls, "claim")
					return testLease, tc.cause
				})
			case "fetch":
				w.fetch = fetchFunc(func(context.Context, submission.Lease) (materials.Descriptor, map[string][]byte, error) {
					*calls = append(*calls, "fetch")
					return materials.Descriptor{}, nil, tc.cause
				})
			case "execute":
				w.execute = executeFunc(func(context.Context, submission.Lease, materials.Descriptor, map[string][]byte) (submission.Result, error) {
					*calls = append(*calls, "execute")
					return testResult, tc.cause
				})
			case "report":
				base := w.report
				w.report = reportFunc(func(ctx context.Context, token string, r submission.Result) error {
					_ = base.Report(ctx, token, r)
					return tc.cause
				})
			}
			if err := w.Run(context.Background()); !errors.Is(err, tc.cause) {
				t.Fatalf("cause lost: %v", err)
			}
			want := []string{"claim"}
			if tc.stage != "claim" {
				want = append(want, "fetch")
			}
			if tc.stage == "execute" || tc.stage == "report" {
				want = append(want, "execute")
			}
			for i := 0; i < tc.reports; i++ {
				want = append(want, "report")
			}
			if !reflect.DeepEqual(*calls, want) {
				t.Fatalf("after failure sequence=%v", *calls)
			}
			if err := w.Run(context.Background()); !errors.Is(err, ErrStarted) {
				t.Fatalf("restarted failed slot: %v", err)
			}
		})
	}
}

func TestIdlePollingAndConcurrentRun(t *testing.T) {
	w, calls := fixture(t)
	w.idleDelay = 30 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	claimed := make(chan time.Time, 2)
	w.claim = claimFunc(func(context.Context) (submission.Lease, error) {
		claimed <- time.Now()
		return submission.Lease{}, submission.ErrNoTask
	})
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	first := <-claimed
	if err := w.Run(ctx); !errors.Is(err, ErrStarted) {
		t.Fatalf("concurrent run: %v", err)
	}
	second := <-claimed
	if second.Sub(first) < w.idleDelay {
		t.Fatal("empty queue busy poll")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Fatal("empty queue invoked downstream operations")
	}
}

func TestCancellationAtStageBoundaries(t *testing.T) {
	for _, stage := range []string{"before", "claim", "fetch", "execute", "report", "retry"} {
		t.Run(stage, func(t *testing.T) {
			w, calls := fixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := []string(nil)
			switch stage {
			case "before":
				cancel()
			case "claim":
				base := w.claim
				w.claim = claimFunc(func(ctx context.Context) (submission.Lease, error) { l, e := base.Claim(ctx); cancel(); return l, e })
				want = []string{"claim"}
			case "fetch":
				base := w.fetch
				w.fetch = fetchFunc(func(ctx context.Context, l submission.Lease) (materials.Descriptor, map[string][]byte, error) {
					d, b, e := base.Fetch(ctx, l)
					cancel()
					return d, b, e
				})
				want = []string{"claim", "fetch"}
			case "execute":
				base := w.execute
				w.execute = executeFunc(func(ctx context.Context, l submission.Lease, d materials.Descriptor, b map[string][]byte) (submission.Result, error) {
					r, e := base.JudgeReport(ctx, l, d, b)
					cancel()
					return r, e
				})
				want = []string{"claim", "fetch", "execute"}
			case "report", "retry":
				base := w.report
				w.report = reportFunc(func(ctx context.Context, token string, r submission.Result) error {
					_ = base.Report(ctx, token, r)
					if stage == "report" {
						cancel()
						return nil
					}
					w.retryDelay = time.Hour
					time.AfterFunc(5*time.Millisecond, cancel)
					return workerclient.ErrReportUnknown
				})
				want = []string{"claim", "fetch", "execute", "report"}
			}
			if err := w.Run(ctx); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*calls, want) {
				t.Fatalf("post-cancel operation: %v", *calls)
			}
		})
	}
}

func TestCancellationDrainsExecutionAndPreservesStopError(t *testing.T) {
	w, calls := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, drain := make(chan struct{}), make(chan struct{})
	w.execute = executeFunc(func(ctx context.Context, _ submission.Lease, _ materials.Descriptor, _ map[string][]byte) (submission.Result, error) {
		close(entered)
		<-ctx.Done()
		<-drain
		return submission.Result{}, errors.Join(ctx.Err(), execution.ErrStopUnconfirmed)
	})
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	<-entered
	cancel()
	select {
	case err := <-done:
		t.Fatalf("released before stop/drain: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	close(drain)
	err := <-done
	if !errors.Is(err, context.Canceled) || !errors.Is(err, execution.ErrStopUnconfirmed) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*calls, []string{"claim", "fetch"}) {
		t.Fatal(*calls)
	}
}

func TestRequiresDependencies(t *testing.T) {
	w, _ := fixture(t)
	for _, tc := range []struct {
		c Claimer
		f Fetcher
		e Executor
		r Reporter
	}{{nil, w.fetch, w.execute, w.report}, {w.claim, nil, w.execute, w.report}, {w.claim, w.fetch, nil, w.report}, {w.claim, w.fetch, w.execute, nil}} {
		if _, err := New(tc.c, tc.f, tc.e, tc.r); err == nil {
			t.Fatal("missing dependency accepted")
		}
	}
}

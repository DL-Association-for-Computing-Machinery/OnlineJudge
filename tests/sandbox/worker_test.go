//go:build sandbox

package sandbox_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/worker"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
)

type lifecycleClaim func(context.Context) (submission.Lease, error)

func (f lifecycleClaim) Claim(ctx context.Context) (submission.Lease, error) { return f(ctx) }

type lifecycleReport func(context.Context, string, submission.Result) error

func (f lifecycleReport) Report(ctx context.Context, token string, r submission.Result) error {
	return f(ctx, token, r)
}

// Reuses the report test's empty disposable database and authenticated clients.
// The worker loop owns sequencing; only fixture creation and sandbox ownership
// remain in the test driver. Controller handlers still share one test process.
func testWorkerLifecycle(t *testing.T, store *postgres.Store, acquirer, downloader *workerclient.Acquirer, renewer *workerclient.Renewer, reporter, retry *workerclient.Reporter, loseReply *atomic.Bool) {
	t.Helper()
	verifyEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows := make(map[string]string)
	for _, tc := range []struct{ verdict, source string }{
		{"AC", "#include <cstdio>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);printf(\"%d\",a+b);}"},
		{"WA", "#include <cstdio>\nint main(){puts(\"wrong\");}"},
	} {
		row, _, err := store.Accept(ctx, "lifecycle-fixture", "m0-training", "lifecycle-fixture-"+tc.verdict,
			submission.Request{MaterialVersion: experiment.PinnedVersion, Language: "gnu++17", Source: tc.source})
		if err != nil {
			t.Fatal(err)
		}
		rows[row.ID] = tc.verdict
	}
	transport := &dockerTransport{}
	sandbox := execution.NewGoJudge(transport, func(context.Context, experiment.ExecutionProfile) error { return nil }) // verified above
	runner := execution.NewLeased(sandbox, renewer.Renew, func(context.Context) error { return errors.New("unexpected interruption requires fixture recovery") })
	claims, reports, accepted := 0, 0, 0
	byToken := make(map[string]submission.Result)
	leaseIDs := make(map[string]string)
	w, err := worker.New(lifecycleClaim(func(ctx context.Context) (submission.Lease, error) {
		l, err := acquirer.Claim(ctx)
		claims++
		if errors.Is(err, submission.ErrNoTask) {
			if accepted != 2 {
				t.Error("queue drained before both acknowledgements")
			}
			cancel()
		}
		if err == nil {
			leaseIDs[l.Token] = l.SubmissionID
		}
		return l, err
	}), downloader, runner, lifecycleReport(func(ctx context.Context, token string, r submission.Result) error {
		reports++
		if prior, ok := byToken[token]; ok && prior != r {
			t.Error("retry changed frozen result")
		}
		byToken[token] = r
		if r.Verdict != rows[leaseIDs[token]] {
			t.Error("incorrect attempt verdict")
		}
		client := reporter
		if reports > 1 {
			client = retry
		}
		err := client.Report(ctx, token, r)
		if err == nil {
			accepted++
		}
		return err
	}))
	if err != nil {
		t.Fatal(err)
	}
	loseReply.Store(true)
	if err := w.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if claims != 3 || reports != 3 || accepted != 2 || len(byToken) != 2 || transport.runs != 5 {
		t.Fatalf("claims=%d reports=%d accepted=%d results=%d sandbox_runs=%d", claims, reports, accepted, len(byToken), transport.runs)
	}
	for id, verdict := range rows {
		got, err := store.Get(context.Background(), "lifecycle-fixture", "m0-training", id)
		if err != nil || got.State != "finished" || got.Verdict == nil || *got.Verdict != verdict {
			t.Fatalf("result not durable: %v", err)
		}
	}
	if err := w.Run(context.Background()); !errors.Is(err, worker.ErrStarted) {
		t.Fatalf("slot reused: %v", err)
	}
	t.Log("two sequential real executions persisted; lost acknowledgement replayed across controllers without rerunning; empty queue cancellation drained the loop")
}

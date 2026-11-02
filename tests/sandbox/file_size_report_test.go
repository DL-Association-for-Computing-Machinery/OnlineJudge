//go:build sandbox

package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/worker"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

const fileSizeSource = `#include <fcntl.h>
#include <unistd.h>
int main(){int fd=open("/tmp/bounded-file-size",O_CREAT|O_EXCL|O_WRONLY,0600);
if(fd<0)return 1;char buffer[65536]={};
for(int i=0;i<17;i++){if(write(fd,buffer,sizeof(buffer))!=sizeof(buffer))return 2;}
close(fd);return 3;}`

type fileSizeObserver struct {
	execution.Sandbox
	points []execution.Observation
}

func (s *fileSizeObserver) Run(ctx context.Context, p experiment.ExecutionProfile, id string, input []byte) (execution.Observation, error) {
	o, err := s.Sandbox.Run(ctx, p, id, input)
	s.points = append(s.points, o)
	return o, err
}

// Keep one Worker, LeasedRunner and container lifetime for both submissions.
func testWorkerFileSizeLifecycle(t *testing.T, store *postgres.Store, pool *pgxpool.Pool, acquirer, downloader *workerclient.Acquirer, renewer *workerclient.Renewer, reporter, retry *workerclient.Reporter) {
	t.Helper()
	verifyEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	identity := func() string {
		t.Helper()
		out, err := exec.CommandContext(ctx, "docker", "--context", "desktop-linux", "inspect", "--format",
			"{{.Id}} {{.State.StartedAt}} {{.RestartCount}} {{.State.Running}} {{.State.OOMKilled}}", container).Output()
		if err != nil {
			t.Fatal(err)
		}
		got := strings.TrimSpace(string(out))
		parts := strings.Fields(got)
		if len(parts) != 5 || len(parts[0]) != 64 || parts[3] != "true" || parts[4] != "false" {
			t.Fatal("unexpected sandbox lifecycle identity")
		}
		return got
	}
	original := identity()
	accept := func(key, source string) (string, error) {
		row, _, err := store.Accept(ctx, "file-size-fixture", "m0-training", "file-size-lifecycle-"+key,
			submission.Request{MaterialVersion: experiment.PinnedVersion, Language: "gnu++17", Source: source})
		return row.ID, err
	}
	first, err := accept("OLE", fileSizeSource)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{first}
	var leases []submission.Lease
	var frozen []submission.Result
	var stopped atomic.Int32
	transport := &dockerTransport{}
	sandbox := execution.NewGoJudge(transport, func(context.Context, experiment.ExecutionProfile) error { return nil }) // verified above
	observed := &fileSizeObserver{Sandbox: sandbox}
	runner := execution.NewLeased(observed, renewer.Renew, func(context.Context) error {
		stopped.Add(1)
		return errors.New("unexpected interruption requires fixture recovery")
	})
	drain := make(chan struct{})
	w, err := worker.New(lifecycleClaim(func(ctx context.Context) (submission.Lease, error) {
		lease, err := acquirer.Claim(ctx)
		if err != nil {
			return lease, err
		}
		if len(leases) >= len(ids) || lease.SubmissionID != ids[len(leases)] {
			return lease, errors.New("file-size claim sequence changed")
		}
		leases = append(leases, lease)
		return lease, nil
	}), downloader, runner, lifecycleReport(func(ctx context.Context, token string, result submission.Result) error {
		index := len(frozen)
		if index >= 2 || index >= len(leases) || token != leases[index].Token || result.Verdict != []string{"OLE", "AC"}[index] {
			return errors.New("file-size report sequence changed")
		}
		if err := reporter.Report(ctx, token, result); err != nil {
			return err
		}
		frozen = append(frozen, result)
		if index == 0 {
			// Only enqueue AC after OLE is durably acknowledged, so sequence
			// does not depend on timestamp precision or FIFO tie breaking.
			id, err := accept("AC", "#include <cstdio>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);printf(\"%d\",a+b);}")
			if err != nil {
				return err
			}
			ids = append(ids, id)
		} else {
			close(drain)
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.RunWithDrain(ctx, drain); err != nil {
		t.Fatal(err)
	}
	if stopped.Load() != 0 || len(leases) != 2 || len(frozen) != 2 || len(ids) != 2 || transport.runs != 5 || identity() != original {
		t.Fatalf("same-slot continuity failed: stops=%d claims=%d reports=%d executions=%d", stopped.Load(), len(leases), len(frozen), transport.runs)
	}
	if len(observed.points) != 3 || observed.points[0].Status != "Output Limit Exceeded" || observed.points[0].Exit != 25 || observed.points[0].OutputTruncated {
		t.Fatal("real SIGXFSZ observation missing or falsely marked truncated")
	}
	for _, point := range observed.points[1:] {
		if point.Status != "Accepted" || point.Exit != 0 || point.OutputTruncated {
			t.Fatal("subsequent AC observations changed")
		}
	}
	for index, lease := range leases {
		if lease.AttemptNumber != 1 || lease.JudgeGeneration != 1 || lease.RecoveryGeneration != 1 {
			t.Fatal("unexpected retry or recovery generation")
		}
		got, err := store.Get(ctx, "file-size-fixture", "m0-training", ids[index])
		if err != nil || got.State != "finished" || got.Verdict == nil || *got.Verdict != frozen[index].Verdict {
			t.Fatalf("file-size result not durable: %v", err)
		}
		var payload []byte
		if err := pool.QueryRow(ctx, `SELECT result_payload FROM judge_attempts WHERE token=$1 AND state='completed'`, lease.Token).Scan(&payload); err != nil {
			t.Fatal(err)
		}
		var stored submission.Result
		if err := json.Unmarshal(payload, &stored); err != nil || stored != frozen[index] {
			t.Fatal("persisted frozen result changed")
		}
		for _, client := range []*workerclient.Reporter{reporter, retry} {
			if err := client.Report(ctx, lease.Token, frozen[index]); err != nil {
				t.Fatal(err)
			}
			changed := frozen[index]
			changed.EvidenceSHA256 = hashBytes([]byte("file-size changed evidence"))
			if err := client.Report(ctx, lease.Token, changed); !errors.Is(err, submission.ErrResultConflict) {
				t.Fatalf("conflicting file-size report accepted: %v", err)
			}
		}
		var after []byte
		if err := pool.QueryRow(ctx, `SELECT result_payload FROM judge_attempts WHERE token=$1`, lease.Token).Scan(&after); err != nil || !bytes.Equal(after, payload) {
			t.Fatal("report replay altered first persisted payload")
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM judge_attempts WHERE submission_id=$1`, ids[index]).Scan(&count); err != nil || count != 1 {
			t.Fatal("file-size result required another attempt")
		}
	}
	var cache map[string]string
	if err := json.Unmarshal(api(t, "GET", "/file", nil), &cache); err != nil || len(cache) != 0 {
		t.Fatal("file-size execution artifacts remain")
	}
	t.Log("same Worker/LeasedRunner/container ID+StartedAt: SIGXFSZ OLE/untruncated then AC, five executions, zero Stop, each one attempt; both-controller replay/conflict and cache cleanup passed")
}

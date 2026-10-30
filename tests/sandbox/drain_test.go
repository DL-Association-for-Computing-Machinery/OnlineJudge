//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/x509"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testWorkerDrain(t *testing.T, store *postgres.Store, pool *pgxpool.Pool, ca *testpki.Authority, endpoint string) {
	t.Helper()
	verifyEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, cert, key := ca.Issue(t, "spiffe://onlinejudge/worker/drain-node/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	id := dualDocker(t, "inspect", "--format", "{{.Id}}", container)
	accept := func(key, source string) string {
		t.Helper()
		row, _, err := store.Accept(ctx, "drain-fixture", "m0-training", key,
			submission.Request{MaterialVersion: experiment.PinnedVersion, Language: "gnu++17", Source: source})
		if err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	const ac = "#include <cstdio>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);printf(\"%d\",a+b);}\n"
	first := accept("worker-drain-current", "#include <cstdio>\n#include <unistd.h>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);usleep(800000);printf(\"%d\",a+b);}\n")
	process := dualWorker(t, ca, cert, key, endpoint, "primary")
	// dualProcess cleanup uses SIGTERM and an independent wait, not this test's
	// canceled context. It never clears a dirty marker after an uncertain stop.
	pid := ""
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		process.alive(t)
		pid = dualPID(t, id)
		if pid != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid == "" {
		t.Fatal("drain did not observe a live contestant")
	}
	// Queue only after the first job is executing, without assuming SQL order.
	second := accept("worker-drain-queued", ac)
	if err := process.cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	waitDrainedProcess(t, process, ctx)
	dualStopped(t, id, pid)
	row, err := store.Get(ctx, "drain-fixture", "m0-training", first)
	if err != nil || row.State != "finished" || row.Verdict == nil || *row.Verdict != "AC" {
		t.Fatal("drain did not preserve the current durable AC result")
	}
	var currentAttempts, queuedAttempts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM judge_attempts WHERE submission_id=$1`, first).Scan(&currentAttempts); err != nil || currentAttempts != 1 {
		t.Fatal("drain repeated the current execution")
	}
	row, err = store.Get(ctx, "drain-fixture", "m0-training", second)
	if err != nil || row.State != "queued" || row.Verdict != nil {
		t.Fatal("draining worker changed the queued job")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM judge_attempts WHERE submission_id=$1`, second).Scan(&queuedAttempts); err != nil || queuedAttempts != 0 {
		t.Fatal("draining worker claimed the queued job")
	}

	// This explicit fixture restart occurs only after confirmed stop and PID
	// absence. The command's own successful cleanup must have released its lock.
	restartDrainedSandbox(t, id)
	process = dualWorker(t, ca, cert, key, endpoint, "primary")
	for {
		row, err = store.Get(ctx, "drain-fixture", "m0-training", second)
		if err != nil {
			t.Fatal(err)
		}
		if row.State == "finished" {
			if row.Verdict == nil || *row.Verdict != "AC" {
				t.Fatal("explicit restart did not complete queued AC")
			}
			break
		}
		process.alive(t)
		select {
		case <-ctx.Done():
			t.Fatal("explicitly restarted worker did not complete queued job")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err := process.cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	waitDrainedProcess(t, process, ctx)
	dualStopped(t, id, "")
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM judge_attempts WHERE submission_id=$1`, second).Scan(&queuedAttempts); err != nil || queuedAttempts != 1 {
		t.Fatal("explicit restart did not execute queued job exactly once")
	}
	t.Log("SIGUSR1 during live execution preserved current AC, left queued job unclaimed, confirmed sandbox/PID stop; explicit restart reused clean slot and completed queued AC, then drained idle")
}

func waitDrainedProcess(t *testing.T, process *dualProcess, ctx context.Context) {
	t.Helper()
	select {
	case err := <-process.done:
		process.stopped = true
		if err != nil {
			t.Fatalf("drain command exit: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("drain command did not finish within budget")
	}
}

func restartDrainedSandbox(t *testing.T, id string) {
	t.Helper()
	dualDocker(t, "start", id)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		cmd := exec.CommandContext(ctx, "docker", "--context", "desktop-linux", "exec", id, "curl", "-fsS", "--max-time", "1", "http://127.0.0.1:5050/version")
		if cmd.Run() == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("explicit sandbox restart was not ready")
}

//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

// This driver is deliberately limited to the already verified, exclusively owned
// offline M0 container. Stopping the whole container sacrifices availability to
// establish a host-level fence; it is not a production or shared-host controller.
func TestLeasedExecutionConfirmsHostStop(t *testing.T) {
	verifyEnvironment(t)
	docker := func(ctx context.Context, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, "docker", append([]string{"--context", "colima-oj-m0"}, args...)...).CombinedOutput()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	idBytes, err := docker(ctx, "inspect", "--format", "{{.Id}}", container)
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(string(idBytes))
	if len(id) != 64 {
		t.Fatal("invalid container identity")
	}
	// Restore only this test container after the interrupted runner is drained.
	// Its new process lifetime must be reverified before any later test submits.
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		if out, err := docker(cleanup, "start", id); err != nil {
			t.Errorf("restore fixture: %v %s", err, out)
		}
		for cleanup.Err() == nil {
			if _, err := docker(cleanup, "exec", id, "curl", "-fsS", "--max-time", "1", "http://127.0.0.1:5050/version"); err == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Error("fixture restart not ready")
	}()
	v, manifest, _ := experiment.SelectFixture(experiment.PinnedVersion)
	source := "#include <unistd.h>\nint main(){sleep(30);}\n"
	d := materials.Descriptor{TaskMaterials: submission.TaskMaterials{Version: v, Manifest: manifest, ManifestSHA256: hashBytes(manifest), Source: source, Language: "gnu++17"}, SourceSHA256: hashBytes([]byte(source))}
	blobs := map[string][]byte{}
	for _, s := range []string{"1 2\n", "3\n", "-7 4\n", "-3\n"} {
		blobs[hashBytes([]byte(s))] = []byte(s)
	}
	host, err := workerhost.OpenM0(ctx)
	if err != nil {
		t.Fatal(err)
	}
	transport := &countingTransport{base: host}
	sandbox := execution.NewGoJudge(transport, host.Verify)
	var observedPID atomic.Int64
	stopped := false
	runner := execution.NewLeased(sandbox, func(context.Context, string) error { return nil }, func(stopCtx context.Context) error {
		if err := host.Stop(stopCtx); err != nil {
			return err
		}
		// Check the observed contestant PID in the VM, beyond the REST client and
		// container state. PID reuse conservatively fails confirmation.
		pid := observedPID.Load()
		if pid <= 1 {
			return errors.New("contestant process was not observed")
		}
		probe := exec.CommandContext(stopCtx, "colima", "--profile", "oj-m0", "ssh", "--", "test", "!", "-e", "/proc/"+strconv.FormatInt(pid, 10))
		// Colima tries to mirror the host cwd; the offline VM deliberately has no
		// repository mount. /tmp exists in both environments.
		probe.Dir = "/tmp"
		if err := probe.Run(); err != nil {
			return errors.New("contestant process exit unconfirmed")
		}
		stopped = true
		return nil
	})
	workCtx, stopWork := context.WithCancel(ctx)
	type outcome struct {
		result execution.Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() { r, e := runner.Judge(workCtx, "synthetic-lease", d, blobs); done <- outcome{r, e} }()
	// Wait for the real contestant process before cancelling; a cancelled request
	// before dispatch would not demonstrate remote process termination.
	for observedPID.Load() == 0 && ctx.Err() == nil {
		out, err := docker(ctx, "top", id, "-eo", "pid,comm")
		if err != nil {
			stopWork()
			<-done
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[1] == "main" {
				if pid, err := strconv.ParseInt(fields[0], 10, 64); err == nil && pid > 1 {
					observedPID.Store(pid)
				}
			}
		}
		select {
		case o := <-done:
			stopWork()
			t.Fatalf("execution ended before cancellation: %v", o.err)
		default:
		}
		if observedPID.Load() == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	stopWork()
	o := <-done
	if observedPID.Load() == 0 || !stopped || !errors.Is(o.err, context.Canceled) || errors.Is(o.err, execution.ErrStopUnconfirmed) || o.result.Verdict != "SE" {
		t.Fatalf("pid=%d stop=%t verdict=%s error=%v", observedPID.Load(), stopped, o.result.Verdict, o.err)
	}
	if transport.runs != 2 || o.result.Points[1].Verdict != "NOT_RUN" {
		t.Fatal("later point executed")
	}
	before := transport.runs
	if _, err := runner.Judge(context.Background(), "synthetic-lease", d, blobs); err == nil || transport.runs != before {
		t.Fatal("quarantined executor reused")
	}
	if err := host.Verify(context.Background(), experiment.PinnedEnvironment()); err == nil {
		t.Fatal("stopped host reused")
	}
	t.Log("observed live contestant; container stopped, PID absent, later point NOT_RUN, old runner quarantined")
}

func hashBytes(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

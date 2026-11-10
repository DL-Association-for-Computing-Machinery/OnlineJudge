//go:build sandbox

package sandbox_test

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func recoverWorkerSlot(t *testing.T, ctx context.Context, worker *dualProcess, slot, id, wrongID string) {
	t.Helper()
	// Deliberately omit all controller/database credentials. Recovery has no
	// application transport and may only terminate the exclusively locked slot.
	env := []string{}
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "OJ_") {
			env = append(env, item)
		}
	}
	env = append(env, "OJ_SANDBOX_EXPERIMENT=1", "OJ_WORKER_M0_DUAL_EXPERIMENT=1", "OJ_WORKER_M0_SLOT="+slot)
	recover := func(expectedID string) (string, error) {
		t.Helper()
		bounded, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()
		cmd := exec.CommandContext(bounded, worker.cmd.Path, "recover", expectedID)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	// Freeze the synthetic worker so it cannot spawn another host CLI between
	// observing descendants and SIGKILL. Recovery itself cannot drain these old
	// host processes; production operators must establish that precondition.
	if err := worker.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		out, err := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(worker.cmd.Process.Pid), "-o", "pid=,stat=").Output()
		fields := strings.Fields(string(out))
		if err != nil || len(fields) != 2 || fields[0] != strconv.Itoa(worker.cmd.Process.Pid) {
			t.Fatal("cannot confirm synthetic worker freeze")
		}
		if strings.Contains(fields[1], "T") {
			break
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			t.Fatal("synthetic worker did not enter stopped state")
		}
		time.Sleep(25 * time.Millisecond)
	}
	children := hostDescendants(t, ctx, worker.cmd.Process.Pid)
	if len(children) == 0 {
		t.Fatal("no in-flight synthetic host CLI observed")
	}
	if out, err := recover(id); err == nil || !strings.Contains(out, "M0 slot already owned") {
		t.Fatal("recovery did not reject live worker ownership")
	}
	if err := worker.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	worker.stopped = true
	select {
	case err := <-worker.done:
		if err == nil {
			t.Fatal("SIGKILL worker exited successfully")
		}
	case <-ctx.Done():
		t.Fatal("SIGKILL worker did not exit")
	}
	for _, pid := range children {
		deadline := time.Now().Add(10 * time.Second)
		for {
			out, err := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "pid=").Output()
			if err != nil && ctx.Err() == nil && len(strings.TrimSpace(string(out))) == 0 {
				// ps returns 1 for no matching process; other errors are not evidence.
				if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
					break
				}
			}
			if time.Now().After(deadline) || ctx.Err() != nil {
				t.Fatal("old synthetic worker host CLI did not exit; keep slot quarantined")
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	t.Logf("confirmed frozen worker and exit of %d observed host descendants before recovery", len(children))
	// Ordinary restart cannot bypass the persisted marker after flock is freed.
	rejectOrdinary := func() {
		t.Helper()
		cmd := exec.CommandContext(ctx, worker.cmd.Path)
		cmd.Env = worker.cmd.Env
		if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "M0 slot quarantined") {
			t.Fatal("ordinary worker reused quarantined slot")
		}
	}
	rejectOrdinary()
	if dualDocker(t, "inspect", "--format", "{{.State.Running}}", id) != "true" {
		t.Fatal("SIGKILL unexpectedly stopped sandbox; recovery did not exercise orphan")
	}
	if _, err := recover(wrongID); err == nil {
		t.Fatal("wrong original ID recovered slot")
	}
	rejectOrdinary()
	if _, err := recover(id); err != nil {
		t.Fatal("original slot recovery failed")
	}
	dualStopped(t, id, "")
	if _, err := recover(id); err != nil {
		t.Fatal("already stopped slot recovery failed")
	}
	dualStopped(t, id, "")
}

func hostDescendants(t *testing.T, ctx context.Context, parent int) []int {
	t.Helper()
	out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=").Output()
	if err != nil {
		t.Fatal("cannot inspect synthetic worker host descendants")
	}
	parents := map[int]int{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			t.Fatal("invalid host process evidence")
		}
		pid, err := strconv.Atoi(fields[0])
		ppid, parentErr := strconv.Atoi(fields[1])
		if err != nil || parentErr != nil || pid <= 0 || ppid < 0 {
			t.Fatal("invalid host process IDs")
		}
		parents[pid] = ppid
	}
	seen := map[int]bool{parent: true}
	var children []int
	for changed := true; changed; {
		changed = false
		for pid, ppid := range parents {
			if seen[ppid] && !seen[pid] {
				seen[pid] = true
				children = append(children, pid)
				changed = true
			}
		}
	}
	return children
}

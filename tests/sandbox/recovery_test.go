//go:build sandbox

package sandbox_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
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
	env = append(env, "OJ_SANDBOX_EXPERIMENT=1", "OJ_WORKER_M0_SLOT="+slot)
	recover := func(expectedID string) (string, error) {
		t.Helper()
		bounded, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()
		cmd := exec.CommandContext(bounded, worker.cmd.Path, "recover", expectedID)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
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

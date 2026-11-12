//go:build sandbox

package sandbox_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestUbuntuHTTPJudgeLoop(t *testing.T) {
	if os.Getenv("OJ_SANDBOX_UBUNTU_HTTP") != "1" {
		t.Skip("run the dedicated Ubuntu VM experiment")
	}
	if os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || runtime.GOOS != "linux" || runtime.GOARCH != "arm64" || os.Geteuid() != 0 {
		t.Fatal("explicit opt-in and dedicated Ubuntu VM owner required")
	}
	testHTTPJudgeLoop(t, experiment.UbuntuVersion)
}

func httpLoopLifetime(t *testing.T, version string) func(*testing.T, *dualProcess) {
	t.Helper()
	if version != experiment.UbuntuVersion {
		id := dualDocker(t, "inspect", "--format", "{{.Id}}", container)
		return func(t *testing.T, w *dualProcess) { drainHTTPLoopWorker(t, w, id) }
	}
	const format = "{{.Id}} {{.State.StartedAt}} {{.State.Running}} {{.State.Pid}}"
	before := strings.Fields(ubuntuDocker(t, "inspect", "--format", format, container))
	if len(before) != 4 || len(before[0]) != 64 || before[2] != "true" {
		t.Fatal("missing original Ubuntu slot lifetime")
	}
	pid, err := strconv.Atoi(before[3])
	if err != nil || pid <= 1 {
		t.Fatal("missing original Ubuntu init PID")
	}
	return func(t *testing.T, w *dualProcess) {
		t.Helper()
		drainHTTPWorker(t, w)
		if got := ubuntuDocker(t, "inspect", "--format", format, before[0]); got != before[0]+" "+before[1]+" false 0" {
			t.Fatal("Ubuntu slot stop not confirmed for original lifetime")
		}
		if _, err := os.Stat("/proc/" + strconv.Itoa(pid)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("original Ubuntu init PID disappearance not confirmed")
		}
		t.Log("same Ubuntu slot lifetime stopped, guest init PID absent")
	}
}

func ubuntuDocker(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/docker", append([]string{"--host=unix:///var/run/docker.sock"}, args...)...)
	cmd.Env = []string{"HOME=/", "PATH=/usr/bin:/bin", "LANG=C"}
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal("fixed local Docker readback failed")
	}
	return strings.TrimSpace(string(raw))
}

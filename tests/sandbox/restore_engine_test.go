//go:build sandbox

package sandbox_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

type restoreEngine bool // zero preserves the Desktop driver

func ubuntuRestoreEngine(t *testing.T, ctx context.Context) restoreEngine {
	t.Helper()
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := workerhost.CheckUbuntuAPlusBHost(probe); err != nil {
		t.Fatalf("Ubuntu A+B restore requires the pinned guest: %v", err)
	}
	return true
}

func (e restoreEngine) command(ctx context.Context, args ...string) *exec.Cmd {
	if !e {
		return exec.CommandContext(ctx, "docker", append([]string{"--context", "desktop-linux"}, args...)...)
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/docker", append([]string{"--host=unix:///var/run/docker.sock"}, args...)...)
	cmd.Env = []string{"HOME=/", "PATH=/usr/bin:/bin", "LANG=C"}
	return cmd
}

func (e restoreEngine) docker(t *testing.T, args ...string) string {
	t.Helper()
	if !e {
		return dualDocker(t, args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := e.command(ctx, args...).Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			t.Fatalf("Ubuntu sandbox control failed: %v: %s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		t.Fatal("Ubuntu sandbox control failed: ", err)
	}
	return strings.TrimSpace(string(out))
}

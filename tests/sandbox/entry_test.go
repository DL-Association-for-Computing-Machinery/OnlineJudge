//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/x509"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

// A real command process uses ephemeral node credentials, real TLS controllers,
// the disposable database and the verified offline container. No database
// credential is passed to the worker. This is still a single-slot experiment.
func testWorkerCommand(t *testing.T, store *postgres.Store, ca *testpki.Authority, endpoint string) {
	t.Helper()
	verifyEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, cert, key := ca.Issue(t, "spiffe://onlinejudge/worker/entry-node/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	binary, err := filepath.Abs("../../.local/bin/worker")
	if err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "OJ_") {
			env = append(env, item)
		}
	}
	env = append(env, "OJ_SANDBOX_EXPERIMENT=1", "OJ_WORKER_CONTROLLER_URL="+endpoint,
		"OJ_WORKER_CA_FILE="+ca.CAFile, "OJ_WORKER_CERT_FILE="+cert, "OJ_WORKER_KEY_FILE="+key)
	docker := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, "docker", append([]string{"--context", "colima-oj-m0"}, args...)...).Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	id := docker("inspect", "--format", "{{.Id}}", container)
	restore := func() {
		t.Helper()
		docker("start", id)
		for ctx.Err() == nil {
			cmd := exec.CommandContext(ctx, "docker", "--context", "colima-oj-m0", "exec", id, "curl", "-fsS", "--max-time", "1", "http://127.0.0.1:5050/version")
			if cmd.Run() == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("sandbox restore timeout")
	}
	accept := func(name, source string) string {
		t.Helper()
		row, _, err := store.Accept(ctx, "entry-fixture", "m0-training", "entry-fixture-"+name,
			submission.Request{MaterialVersion: experiment.PinnedVersion, Language: "gnu++17", Source: source})
		if err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	rows := map[string]string{
		accept("AC", "#include <cstdio>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);printf(\"%d\",a+b);}"): "AC",
		accept("WA", "#include <cstdio>\nint main(){puts(\"wrong\");}"):                                  "WA",
	}
	start := func() (*exec.Cmd, <-chan error) {
		t.Helper()
		log, err := os.CreateTemp(t.TempDir(), "worker-log-")
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, binary)
		cmd.Env = env
		cmd.Stdout = log
		cmd.Stderr = log
		if err := cmd.Start(); err != nil {
			log.Close()
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait(); log.Close(); close(done) }()
		t.Cleanup(func() {
			cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-done:
			case <-time.After(15 * time.Second):
				cmd.Process.Kill()
				<-done
			}
		})
		return cmd, done
	}
	stop := func(cmd *exec.Cmd, done <-chan error, signal os.Signal) {
		t.Helper()
		if err := cmd.Process.Signal(signal); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("worker exit: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("worker did not drain")
		}
		if got := docker("inspect", "--format", "{{.State.Running}} {{.State.Pid}} {{.State.Restarting}}", id); got != "false 0 false" {
			t.Fatalf("stop unconfirmed: %s", got)
		}
	}
	cmd, done := start()
	for len(rows) > 0 && ctx.Err() == nil {
		for id, want := range rows {
			row, err := store.Get(ctx, "entry-fixture", "m0-training", id)
			if err != nil {
				t.Fatal(err)
			}
			if row.State == "finished" {
				if row.Verdict == nil || *row.Verdict != want {
					t.Fatal("wrong command result")
				}
				delete(rows, id)
			}
		}
		select {
		case err := <-done:
			t.Fatalf("worker stopped early: %v", err)
		default:
		}
		time.Sleep(30 * time.Millisecond)
	}
	if len(rows) != 0 {
		t.Fatal("command results not persisted")
	}
	second := exec.CommandContext(ctx, binary)
	second.Env = env
	out, err := second.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "M0 slot already owned") {
		t.Fatal("duplicate command was not rejected by ownership lock")
	}
	stop(cmd, done, os.Interrupt)
	restore()
	verifyEnvironment(t)
	idleRow := accept("signal", "#include <unistd.h>\nint main(){sleep(30);}\n")
	cmd, done = start()
	pid := ""
	for pid == "" && ctx.Err() == nil {
		for _, line := range strings.Split(docker("top", id, "-eo", "pid,comm"), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[1] == "main" {
				pid = fields[0]
			}
		}
		select {
		case err := <-done:
			t.Fatalf("worker stopped before signal: %v", err)
		default:
		}
		if pid == "" {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if pid == "" {
		t.Fatal("no live contestant observed")
	}
	stop(cmd, done, syscall.SIGTERM)
	probe := exec.CommandContext(ctx, "colima", "--profile", "oj-m0", "ssh", "--", "test", "!", "-e", "/proc/"+pid)
	probe.Dir = "/tmp"
	if err := probe.Run(); err != nil {
		t.Fatal("contestant process still present")
	}
	row, err := store.Get(ctx, "entry-fixture", "m0-training", idleRow)
	if err != nil || row.Verdict != nil || row.State == "finished" {
		t.Fatal("interrupted execution published a verdict")
	}
	restore()
	t.Log("real worker process persisted AC/WA; duplicate process rejected; SIGINT idle and SIGTERM live contestant stopped container; PID absent and interrupted result unpublished")
}

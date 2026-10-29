//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

type dualProcess struct {
	cmd     *exec.Cmd
	done    chan error
	stopped bool
}

func startDualProcess(t *testing.T, name string, env []string) *dualProcess {
	t.Helper()
	binary, err := filepath.Abs("../../.local/bin/" + name)
	if err != nil {
		t.Fatal(err)
	}
	log, err := os.CreateTemp(t.TempDir(), name+"-log-")
	if err != nil {
		t.Fatal(err)
	}
	p := &dualProcess{cmd: exec.Command(binary), done: make(chan error, 1)}
	p.cmd.Env, p.cmd.Stdout, p.cmd.Stderr = env, log, log
	if err := p.cmd.Start(); err != nil {
		_ = log.Close() // Cleanup; the operation outcome is checked separately.
		t.Fatal(err)
	}
	go func() { p.done <- p.cmd.Wait(); _ = log.Close() }() // Process status is reported through done.
	t.Cleanup(func() {
		if !p.stopped {
			p.stop(t)
		}
	})
	return p
}

func (p *dualProcess) stop(t *testing.T) {
	t.Helper()
	if p.stopped {
		t.Fatal("process already stopped")
	}
	p.stopped = true
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case err := <-p.done:
		if err != nil {
			t.Errorf("experiment process exit: %v", err)
		}
	case <-time.After(15 * time.Second):
		_ = p.cmd.Process.Kill()
		err := <-p.done
		t.Errorf("experiment process failed to drain: %v", err)
	}
}

func (p *dualProcess) alive(t *testing.T) {
	t.Helper()
	select {
	case err := <-p.done:
		p.stopped = true
		t.Fatalf("experiment process stopped early: %v", err)
	default:
	}
}

func dualController(t *testing.T, ca *testpki.Authority, clientCert tls.Certificate, root string) string {
	t.Helper()
	_, cert, key := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close() // Cleanup; the operation outcome is checked separately.
	p := startDualProcess(t, "worker-api", []string{
		"OJ_WORKER_API_ADDR=" + addr, "OJ_WORKER_CA_FILE=" + ca.CAFile,
		"OJ_WORKER_CERT_FILE=" + cert, "OJ_WORKER_KEY_FILE=" + key,
		"OJ_DATABASE_URL=" + os.Getenv("OJ_TEST_DATABASE_URL"), "OJ_MATERIAL_ROOT=" + root,
		"OJ_EXPERIMENT_MATERIAL_VERSION=" + experiment.PinnedVersion,
	})
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: ca.Roots, Certificates: []tls.Certificate{clientCert}}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	endpoint := "https://" + addr
	for range 50 {
		p.alive(t)
		resp, err := client.Get(endpoint + "/not-a-route")
		if err == nil {
			_ = resp.Body.Close() // Cleanup; the operation outcome is checked separately.
			if resp.StatusCode == http.StatusNotFound {
				return endpoint
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("controller readiness timeout")
	return ""
}

func dualWorker(t *testing.T, ca *testpki.Authority, cert, key, endpoint, slot string) *dualProcess {
	t.Helper()
	var env []string
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "OJ_") {
			env = append(env, item)
		}
	}
	return startDualProcess(t, "worker", append(env,
		"OJ_SANDBOX_EXPERIMENT=1", "OJ_WORKER_CONTROLLER_URL="+endpoint,
		"OJ_WORKER_CA_FILE="+ca.CAFile, "OJ_WORKER_CERT_FILE="+cert,
		"OJ_WORKER_KEY_FILE="+key, "OJ_WORKER_M0_SLOT="+slot))
}

func dualDocker(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", append([]string{"--context", "desktop-linux"}, args...)...).Output()
	if err != nil {
		t.Fatal("sandbox control failed: ", err)
	}
	return strings.TrimSpace(string(out))
}

func dualPID(t *testing.T, id string) string {
	t.Helper()
	for _, line := range strings.Split(dualDocker(t, "top", id, "-eo", "pid,comm"), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == "main" {
			return fields[0]
		}
	}
	return ""
}

func dualStopped(t *testing.T, id, pid string) {
	t.Helper()
	if dualDocker(t, "inspect", "--format", "{{.State.Running}} {{.State.Pid}} {{.State.Restarting}}", id) != "false 0 false" {
		t.Fatal("sandbox stop unconfirmed")
	}
	if pid != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", workerhost.PIDProbeArgs(pid)...)
		cmd.Dir = "/tmp"
		if err := cmd.Run(); err != nil {
			t.Fatal("contestant PID still present")
		}
	}
}

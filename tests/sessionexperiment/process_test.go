package sessionexperiment

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"
)

type processReady struct {
	URL  string
	Cert []byte
	PID  int
}

// Re-execute the race-instrumented test binary, keeping all endpoints test-only.
// Control messages use pipes, never an unauthenticated HTTP control endpoint.
func TestSessionAPIProcess(t *testing.T) {
	prefix := os.Getenv("OJ_SESSION_PROCESS_PREFIX")
	if prefix == "" {
		t.Skip("child process entry point")
	}
	f := setupReplicas(t, prefix, 1)
	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(processReady{f.servers[0].URL, f.servers[0].Certificate().Raw, os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	commands := bufio.NewScanner(os.Stdin)
	for commands.Scan() {
		switch commands.Text() {
		case "loaded":
			select {
			case <-f.loaded:
			case <-time.After(5 * time.Second):
				t.Fatal("child load barrier timed out")
			}
		case "resume":
			close(f.resume)
		default:
			t.Fatal("unknown process control command")
		}
		if err := encoder.Encode("ok"); err != nil {
			t.Fatal(err)
		}
	}
	if err := commands.Err(); err != nil {
		t.Fatal(err)
	}
}

type apiProcess struct {
	cmd    *exec.Cmd
	input  io.WriteCloser
	output *json.Decoder
	done   chan error
	client *http.Client
	url    string
	stop   sync.Once
}

func startAPIProcess(t *testing.T, prefix string) *apiProcess {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestSessionAPIProcess$", "-test.timeout=25s")
	cmd.Env = append(os.Environ(), "OJ_SESSION_PROCESS_PREFIX="+prefix, "GORACE=halt_on_error=1")
	cmd.Stderr = os.Stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &apiProcess{cmd: cmd, input: input, output: json.NewDecoder(output), done: make(chan error, 1)}
	go func() { p.done <- cmd.Wait() }()
	t.Cleanup(func() {
		p.stop.Do(func() {
			_ = p.input.Close()
			select {
			case err := <-p.done:
				if err != nil {
					t.Errorf("API process exit: %v", err)
				}
			case <-time.After(5 * time.Second):
				_ = p.cmd.Process.Kill()
				err := <-p.done
				t.Errorf("API process did not exit after control pipe closed: %v", err)
			}
		})
	})
	var ready processReady
	if err := p.output.Decode(&ready); err != nil {
		t.Fatalf("API readiness: %v", err)
	}
	if ready.PID != cmd.Process.Pid || ready.PID == os.Getpid() {
		t.Fatal("API is not the expected independent process")
	}
	cert, err := x509.ParseCertificate(ready.Cert)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	t.Cleanup(transport.CloseIdleConnections)
	p.client = &http.Client{Transport: transport, Timeout: 5 * time.Second}
	p.url = ready.URL
	return p
}

func (p *apiProcess) kill(t *testing.T) {
	t.Helper()
	p.stop.Do(func() {
		if err := p.cmd.Process.Kill(); err != nil {
			t.Errorf("kill API process: %v", err)
		}
		_ = p.input.Close()
		select {
		case err := <-p.done:
			exit, ok := err.(*exec.ExitError)
			if !ok {
				t.Errorf("API process did not report forced exit: %v", err)
			} else if status, ok := exit.Sys().(syscall.WaitStatus); !ok || status.Signal() != syscall.SIGKILL {
				t.Errorf("API process did not exit from SIGKILL: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("API process did not exit after kill")
		}
	})
}

func (p *apiProcess) control(t *testing.T, command string) {
	t.Helper()
	if _, err := fmt.Fprintln(p.input, command); err != nil {
		t.Fatal(err)
	}
	var result string
	if err := p.output.Decode(&result); err != nil || result != "ok" {
		t.Fatalf("process barrier %s: %v", command, err)
	}
}

func (p *apiProcess) request(method, path string, cookie *http.Cookie) (*http.Response, error) {
	req, err := http.NewRequest(method, p.url+path, nil)
	if err != nil {
		return nil, err
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	response, err := p.client.Do(req)
	if err == nil {
		_ = response.Body.Close() // Cleanup; the operation outcome is checked separately.
	}
	return response, err
}

func (p *apiProcess) expect(t *testing.T, method, path string, cookie *http.Cookie, status int) *http.Response {
	t.Helper()
	r, err := p.request(method, path, cookie)
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != status {
		t.Fatalf("%s: got %d want %d", path, r.StatusCode, status)
	}
	return r
}

func TestIndependentAPIProcessesAndRestart(t *testing.T) {
	prefix := "experiment:" + digest(t.Name()) + ":"
	apis := [2]*apiProcess{startAPIProcess(t, prefix), startAPIProcess(t, prefix)}
	if apis[0].cmd.Process.Pid == apis[1].cmd.Process.Pid {
		t.Fatal("API processes share PID")
	}
	start := make(chan struct{})
	results := make(chan *http.Response, 24)
	errors := make(chan error, 24)
	var workers sync.WaitGroup
	for i := range 24 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			r, err := apis[i%2].request("POST", "/login-fixture", nil)
			if err != nil {
				errors <- err
				return
			}
			results <- r
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	var cookies []*http.Cookie
	conflicts := 0
	for r := range results {
		switch r.StatusCode {
		case 201:
			if len(r.Cookies()) != 1 {
				t.Fatal("admission must send exactly one cookie")
			}
			cookies = append(cookies, r.Cookies()[0])
		case 409:
			conflicts++
			if len(r.Cookies()) != 0 {
				t.Fatal("failed admission sent cookie")
			}
		default:
			t.Fatalf("unexpected admission status %d", r.StatusCode)
		}
	}
	if len(cookies) != 3 || conflicts != 21 {
		t.Fatalf("admitted=%d rejected=%d", len(cookies), conflicts)
	}
	for _, api := range apis {
		for _, cookie := range cookies {
			api.expect(t, "GET", "/me", cookie, 200)
		}
	}
	late := make(chan error, 1)
	go func() {
		r, err := apis[0].request("POST", "/late-save", cookies[0])
		if err == nil && (r.StatusCode != 401 || len(r.Cookies()) != 0) {
			err = fmt.Errorf("late save status=%d cookies=%d", r.StatusCode, len(r.Cookies()))
		}
		late <- err
	}()
	apis[0].control(t, "loaded")
	apis[1].expect(t, "POST", "/logout", cookies[0], 204)
	apis[0].control(t, "resume")
	if err := <-late; err != nil {
		t.Fatal(err)
	}
	// Kill the process that performed revocation. Redis stays alive throughout.
	apis[1].kill(t)
	apis[0].expect(t, "GET", "/me", cookies[0], 401)
	apis[0].expect(t, "GET", "/me", cookies[1], 200)
	apis[1] = startAPIProcess(t, prefix)
	for _, api := range apis {
		api.expect(t, "GET", "/me", cookies[0], 401)
		for _, cookie := range cookies[1:] {
			api.expect(t, "GET", "/me", cookie, 200)
		}
	}
	replacement := apis[1].expect(t, "POST", "/login-fixture", nil, 201).Cookies()[0]
	apis[0].expect(t, "GET", "/me", replacement, 200)
	for _, api := range apis {
		api.expect(t, "POST", "/login-fixture", nil, 409)
	}
}

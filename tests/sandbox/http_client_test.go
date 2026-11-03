//go:build sandbox

package sandbox_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

type loopAPI struct {
	process  *dualProcess
	endpoint string
}

type loopRequest struct {
	method, path, key, auth, origin string
	body                            []byte
	status                          int
}

type loopResponse struct {
	body     []byte
	location string
}

func httpLoopEnvironment(extra ...string) []string {
	var env []string
	for _, name := range []string{"PATH", "HOME", "TMPDIR", "DOCKER_CONFIG"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return append(env, extra...)
}

func migrateHTTPLoop(t *testing.T, ctx context.Context, dsn, root string) {
	t.Helper()
	binary, err := filepath.Abs("../../.local/bin/migrate")
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range [][]string{{"up"}, {"fixture"}, {"backlog", "10000", "10000"}} {
		log, err := os.CreateTemp(t.TempDir(), "migration-log-")
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, binary, step...)
		cmd.Env = httpLoopEnvironment("OJ_DATABASE_URL="+dsn, "OJ_MATERIAL_ROOT="+root,
			"OJ_EXPERIMENT_MATERIAL_VERSION="+experiment.PinnedVersion)
		cmd.Stdout, cmd.Stderr = log, log
		runErr := cmd.Run()
		closeErr := log.Close()
		if runErr != nil || closeErr != nil {
			t.Fatal("private HTTP fixture migration failed")
		}
	}
}

func startHTTPLoopAPI(t *testing.T, ctx context.Context, client *http.Client, dsn, tokenFile string, options ...string) loopAPI {
	t.Helper()
	env := httpLoopEnvironment("OJ_API_ADDR=127.0.0.1:0", "OJ_DATABASE_URL="+dsn,
		"OJ_EXPERIMENT_TOKEN_FILE="+tokenFile, "OJ_EXPERIMENT_MATERIAL_VERSION="+experiment.PinnedVersion)
	p := startDualProcess(t, "api", append(env, options...))
	log, ok := p.cmd.Stdout.(*os.File)
	if !ok {
		t.Fatal("API process does not own a private log")
	}
	startup, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	for startup.Err() == nil {
		p.alive(t)
		file, err := os.Open(log.Name())
		if err != nil {
			t.Fatal("cannot read private API startup log")
		}
		data, readErr := io.ReadAll(io.LimitReader(file, apiLogLimit+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			t.Fatal("cannot read bounded API startup log")
		}
		endpoint, found, err := experimentAPIAddress(data)
		if err != nil {
			t.Fatal(err)
		}
		if found {
			req, err := http.NewRequestWithContext(startup, http.MethodGet, endpoint+"/api/v1/healthz", nil)
			if err != nil {
				t.Fatal("invalid private API health request")
			}
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close() // Readiness checks status, not application results.
				if resp.StatusCode != http.StatusOK {
					t.Fatal("unexpected private API health status")
				}
				p.alive(t)
				return loopAPI{p, endpoint}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("API startup exceeded observation deadline")
	return loopAPI{}
}

func sendLoopHTTP(t *testing.T, ctx context.Context, client *http.Client, api loopAPI, call loopRequest) loopResponse {
	t.Helper()
	api.process.alive(t)
	req, err := http.NewRequestWithContext(ctx, call.method, api.endpoint+call.path, bytes.NewReader(call.body))
	if err != nil {
		t.Fatal("invalid controlled HTTP request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", call.key)
	if call.auth != "" {
		req.Header.Set("Authorization", call.auth)
	}
	if call.origin != "" {
		req.Header.Set("Origin", call.origin)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("controlled API request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }() // Body is consumed below; transport closes after all processes.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil || len(data) > 4096 {
		t.Fatal("invalid or unbounded API response")
	}
	if resp.StatusCode != call.status || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Access-Control-Allow-Origin") != "" || len(resp.Header.Values("Set-Cookie")) != 0 {
		t.Fatalf("HTTP boundary: status=%d want=%d", resp.StatusCode, call.status)
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	wantType := "application/json"
	if call.status >= 400 {
		wantType = "application/problem+json"
	}
	if err != nil || contentType != wantType {
		t.Fatal("unexpected API media type")
	}
	api.process.alive(t)
	return loopResponse{data, resp.Header.Get("Location")}
}

func loopReceipt(t *testing.T, response loopResponse) submission.Receipt {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.body, &fields); err != nil || len(fields) != 6 {
		t.Fatal("receipt does not match closed public schema")
	}
	for _, name := range []string{"id", "material_version", "language", "created_at", "state", "verdict"} {
		if _, ok := fields[name]; !ok {
			t.Fatal("receipt field missing or unexpected private field present")
		}
	}
	var receipt submission.Receipt
	if err := json.Unmarshal(response.body, &receipt); err != nil {
		t.Fatal("invalid public receipt")
	}
	id, err := hex.DecodeString(receipt.ID)
	if err != nil || len(id) != 16 || hex.EncodeToString(id) != receipt.ID || receipt.MaterialVersion != experiment.PinnedVersion || receipt.Language != "gnu++17" || receipt.CreatedAt.IsZero() {
		t.Fatal("receipt identity or pinned metadata invalid")
	}
	if response.location != "" && response.location != "/api/v1/experiments/submissions/"+receipt.ID {
		t.Fatal("API returned unexpected or non-relative Location")
	}
	return receipt
}

func loopProblem(t *testing.T, response loopResponse, status int, code string) {
	t.Helper()
	var got struct {
		Status int    `json:"status"`
		Code   string `json:"code"`
	}
	if err := json.Unmarshal(response.body, &got); err != nil || got.Status != status || got.Code != code || response.location != "" {
		t.Fatal("unexpected controlled API error")
	}
}

func drainHTTPLoopWorker(t *testing.T, w *dualProcess, id string) {
	t.Helper()
	w.alive(t)
	if err := w.cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		t.Fatal("cannot request Worker drain")
	}
	select {
	case err := <-w.done:
		w.stopped = true
		if err != nil {
			t.Fatal("HTTP loop Worker did not exit normally")
		}
	case <-time.After(15 * time.Second):
		_ = w.cmd.Process.Kill()
		select {
		case exitErr := <-w.done:
			w.stopped = true
			t.Fatalf("Worker missed drain deadline; kill exit error present=%t", exitErr != nil)
		case <-time.After(5 * time.Second):
			t.Fatal("Worker exit remains unconfirmed after kill")
		}
	}
	dualStopped(t, id, "")
}

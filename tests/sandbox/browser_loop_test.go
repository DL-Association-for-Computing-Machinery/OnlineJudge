//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func startBrowserDriver(t *testing.T, api loopAPI, otherKey, out string) *dualProcess {
	t.Helper()
	node, module := os.Getenv("OJ_BROWSER_NODE"), os.Getenv("OJ_PLAYWRIGHT_MODULE")
	if node == "" || module == "" || !filepath.IsAbs(node) || !filepath.IsAbs(module) {
		t.Fatal("existing explicit browser tools required")
	}
	script, err := filepath.Abs("../browser/m0-qa.mjs")
	if err != nil {
		t.Fatal(err)
	}
	log, err := os.OpenFile(filepath.Join(out, "browser.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("cannot create private browser log")
	}
	p := &dualProcess{cmd: exec.Command(node, script), done: make(chan error, 1)}
	p.cmd.Env = httpLoopEnvironment("OJ_PLAYWRIGHT_MODULE="+module, "OJ_BROWSER_OUTPUT="+out,
		"OJ_BROWSER_ORIGIN="+api.endpoint, "OJ_BROWSER_OTHER_KEY="+otherKey,
		"OJ_BROWSER_CHROMIUM="+os.Getenv("OJ_BROWSER_CHROMIUM"))
	p.cmd.Stdout, p.cmd.Stderr = log, log
	if err := p.cmd.Start(); err != nil {
		_ = log.Close()
		t.Fatal("cannot start private browser driver")
	}
	go func() { p.done <- p.cmd.Wait(); _ = log.Close() }()
	t.Cleanup(func() {
		if !p.stopped {
			p.stop(t)
		}
	})
	return p
}

func TestBrowserJudgeLoop(t *testing.T) {
	if os.Getenv("OJ_SANDBOX_BROWSER") != "1" {
		t.Skip("run the explicit browser-loop driver with existing Playwright")
	}
	verifyEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	out := os.Getenv("OJ_BROWSER_OUTPUT")
	info, err := os.Stat(out)
	if err != nil || !info.IsDir() || !filepath.IsAbs(out) {
		t.Fatal("new external browser output directory required")
	}
	dsn := os.Getenv("OJ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("private disposable database required")
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("cannot open private browser observer")
	}
	conn.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = conn.Close() })
	var tables int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatal("browser loop requires an empty database")
	}
	root := t.TempDir()
	migrateHTTPLoop(t, ctx, dsn, root)
	if _, err := conn.ExecContext(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)`, "browser-loop-node"); err != nil {
		t.Fatal("cannot provision synthetic browser Worker")
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		t.Fatal("cannot prepare private experiment credential")
	}
	token := hex.EncodeToString(secret[:])
	tokenFile := filepath.Join(t.TempDir(), "private-experiment-token")
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		t.Fatal("cannot store private experiment credential")
	}
	transport := &http.Transport{Proxy: nil}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	api := startHTTPLoopAPI(t, ctx, client, dsn, tokenFile, "OJ_EXPERIMENT_BROWSER=1")
	const otherKey = "m0-browser-private-scope-key"
	const sumSource = "#include <cstdio>\n\nint main() {\n    int a, b;\n    scanf(\"%d%d\", &a, &b);\n    printf(\"%d\", a + b);\n}"
	const wrongSource = "#include <cstdio>\n\nint main() {\n    puts(\"wrong\");\n}"
	body, err := json.Marshal(submission.Request{MaterialVersion: experiment.PinnedVersion, Language: "gnu++17", Source: sumSource})
	if err != nil {
		t.Fatal(err)
	}
	other := loopReceipt(t, sendLoopHTTP(t, ctx, client, api, loopRequest{method: "POST", path: "/api/v1/experiments/submissions", key: otherKey, auth: "Bearer " + token, body: body, status: 202}))
	if other.State != "queued" {
		t.Fatal("private-scope fixture was not accepted")
	}
	loopProblem(t, sendLoopHTTP(t, ctx, client, api, loopRequest{method: "POST", path: "/api/v1/experiments/submissions", key: otherKey, auth: "Bearer " + token, origin: api.endpoint, body: body, status: 403}), 403, "EXPERIMENT_ONLY")
	browser := startBrowserDriver(t, api, otherKey, out)
	ready := false
	for deadline := time.Now().Add(45 * time.Second); time.Now().Before(deadline) && ctx.Err() == nil; {
		browser.alive(t)
		data, err := os.ReadFile(filepath.Join(out, "queued.json"))
		if err == nil {
			var marker struct {
				ID, Key string
			}
			if len(data) > 1024 || json.Unmarshal(data, &marker) != nil || len(marker.ID) != 32 || !submission.ValidKey(marker.Key) {
				t.Fatal("invalid browser queued observation")
			}
			var state, subject string
			var submissions, tasks, attempts int
			if err := conn.QueryRowContext(ctx, `SELECT t.state,s.subject_id,(SELECT count(*) FROM submissions),(SELECT count(*) FROM judge_tasks),(SELECT count(*) FROM judge_attempts) FROM submissions s JOIN judge_tasks t ON t.submission_id=s.id WHERE s.id=$1 AND s.context_id=$2 AND s.idempotency_key=$3`, marker.ID, "m0-browser-training", marker.Key).Scan(&state, &subject, &submissions, &tasks, &attempts); err != nil || state != "queued" || subject != "m0-browser-operator" || submissions != 2 || tasks != 2 || attempts != 0 {
				t.Fatal("browser queued state or lost-receipt exactly-once evidence missing before Worker start")
			}
			ready = true
			break
		}
		if !os.IsNotExist(err) {
			t.Fatal("cannot read private browser observation")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatal("browser queued observation timed out")
	}
	ca := testpki.New(t)
	cert, certFile, keyFile := ca.Issue(t, "spiffe://onlinejudge/worker/browser-loop-node/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	controller := dualController(t, ca, cert, root)
	id := dualDocker(t, "inspect", "--format", "{{.Id}}", container)
	w := startDualProcess(t, "worker", httpLoopEnvironment("OJ_SANDBOX_EXPERIMENT=1", "OJ_WORKER_M0_SLOT=primary", "OJ_WORKER_CONTROLLER_URL="+controller,
		"OJ_WORKER_CA_FILE="+ca.CAFile, "OJ_WORKER_CERT_FILE="+certFile, "OJ_WORKER_KEY_FILE="+keyFile))
	select {
	case err := <-browser.done:
		browser.stopped = true
		if err != nil {
			t.Fatal("browser assertions failed; inspect private browser artifacts")
		}
	case <-ctx.Done():
		t.Fatal("browser workflow exceeded deadline")
	}
	data, err := os.ReadFile(filepath.Join(out, "result.json"))
	var result struct {
		Success    bool                               `json:"success"`
		Operations []struct{ ID, Key, Preset string } `json:"operations"`
	}
	if err != nil || len(data) > 65536 || json.Unmarshal(data, &result) != nil || !result.Success || len(result.Operations) != 2 {
		t.Fatal("browser result contract missing")
	}
	for i, op := range result.Operations {
		wantPreset, wantVerdict, wantSource := "sum", "AC", sumSource
		if i == 1 {
			wantPreset, wantVerdict, wantSource = "wrong", "WA", wrongSource
		}
		var source, verdict, node, attemptState string
		var attempt, judge, recovery int
		if op.Preset != wantPreset || !submission.ValidKey(op.Key) {
			t.Fatal("browser operation identity changed")
		}
		if err := conn.QueryRowContext(ctx, `SELECT s.source_code,t.verdict,a.node_id,a.state,a.attempt_number,a.judge_generation,a.recovery_generation FROM submissions s JOIN judge_tasks t ON t.submission_id=s.id JOIN judge_attempts a ON a.submission_id=s.id WHERE s.id=$1 AND s.idempotency_key=$2 AND s.subject_id=$3 AND s.context_id=$4`, op.ID, op.Key, "m0-browser-operator", "m0-browser-training").Scan(&source, &verdict, &node, &attemptState, &attempt, &judge, &recovery); err != nil || source != wantSource || verdict != wantVerdict || node != "browser-loop-node" || attemptState != "completed" || attempt != 1 || judge != 1 || recovery != 1 {
			t.Fatal("browser source attribution or one real completion not preserved")
		}
	}
	var submissions, tasks, attempts int
	if err := conn.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM submissions),(SELECT count(*) FROM judge_tasks),(SELECT count(*) FROM judge_attempts)`).Scan(&submissions, &tasks, &attempts); err != nil || submissions != 3 || tasks != 3 || attempts != 3 {
		t.Fatal("negative browser requests or original-key retries created extra work")
	}
	var finished, completed int
	if err := conn.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM judge_tasks WHERE state='finished'),(SELECT count(*) FROM judge_attempts WHERE state='completed' AND attempt_number=1 AND judge_generation=1 AND recovery_generation=1)`).Scan(&finished, &completed); err != nil || finished != 3 || completed != 3 {
		t.Fatal("all three synthetic tasks must finish with one completed attempt each")
	}
	drainHTTPLoopWorker(t, w, id)
	api.process.stop(t)
	t.Log("real browser→API→mTLS controller→Worker→browser AC/WA; actual lost acceptance response/original-key recovery, separate Bearer scope, desktop/mobile and normal drain passed")
}

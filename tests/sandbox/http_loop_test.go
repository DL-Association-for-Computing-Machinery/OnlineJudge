//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
)

func TestHTTPJudgeLoop(t *testing.T) {
	if os.Getenv("OJ_SANDBOX_HTTP") != "1" {
		t.Skip("run python3 scripts/test-worker.py http-loop")
	}
	verifyEnvironment(t)
	testHTTPJudgeLoop(t, experiment.PinnedVersion)
}

func testHTTPJudgeLoop(t *testing.T, version string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	dsn := os.Getenv("OJ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("private disposable database required")
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("cannot open private observer connection")
	}
	conn.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = conn.Close() }) // Processes registered later close first.
	var tables int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatal("HTTP loop database must be empty")
	}
	root := t.TempDir()
	migrateHTTPFixture(t, ctx, dsn, root, version, "2")
	if _, err := conn.ExecContext(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)`, "http-loop-node"); err != nil {
		t.Fatal("cannot provision synthetic node")
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		t.Fatal("cannot prepare private fixture credential")
	}
	token := hex.EncodeToString(secret[:])
	tokenFile := filepath.Join(t.TempDir(), "experiment-token")
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		t.Fatal("cannot store private fixture credential")
	}
	transport := &http.Transport{Proxy: nil, MaxConnsPerHost: 2, MaxIdleConnsPerHost: 2}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	option := "OJ_EXPERIMENT_MATERIAL_VERSION=" + version
	apis := []loopAPI{startHTTPLoopAPI(t, ctx, client, dsn, tokenFile, option), startHTTPLoopAPI(t, ctx, client, dsn, tokenFile, option)}
	if apis[0].endpoint == apis[1].endpoint {
		t.Fatal("submission APIs must be separate processes/listeners")
	}
	request := func(index int, method, path, key string, body []byte, status int) loopResponse {
		t.Helper()
		return sendLoopHTTP(t, ctx, client, apis[index], loopRequest{method: method, path: path, key: key,
			body: body, auth: "Bearer " + token, status: status})
	}
	counts := func(submissions, tasks, attempts int) {
		t.Helper()
		var a, b, c int
		if err := conn.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM submissions),(SELECT count(*) FROM judge_tasks),(SELECT count(*) FROM judge_attempts)`).Scan(&a, &b, &c); err != nil || a != submissions || b != tasks || c != attempts {
			t.Fatalf("unexpected fixture side effects: submissions=%d tasks=%d attempts=%d", a, b, c)
		}
	}
	type judgeCase struct {
		key, verdict, source string
		body                 []byte
		queued               submission.Receipt
	}
	cases := []judgeCase{
		{key: "http-loop-ac-key-0001", verdict: "AC", source: "#include <cstdio>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);printf(\"%d\",a+b);}"},
		{key: "http-loop-wa-key-0002", verdict: "WA", source: "#include <cstdio>\nint main(){puts(\"wrong\");}"},
	}
	for i := range cases {
		if !submission.ValidKey(cases[i].key) {
			t.Fatal("controlled idempotency key invalid")
		}
		body, err := json.Marshal(submission.Request{MaterialVersion: version, Language: "gnu++17", Source: cases[i].source})
		if err != nil {
			t.Fatal("cannot encode controlled source")
		}
		cases[i].body = body
	}
	for _, denied := range []struct {
		name, auth, origin, code string
		status                   int
	}{
		{"no-auth", "", "", "UNAUTHENTICATED", 401},
		{"wrong-auth", "Bearer incorrect-fixture-credential-00000000", "", "UNAUTHENTICATED", 401},
		{"browser-origin", "Bearer " + token, "https://example.invalid", "EXPERIMENT_ONLY", 403},
	} {
		response := sendLoopHTTP(t, ctx, client, apis[0], loopRequest{method: "POST", path: "/api/v1/experiments/submissions",
			key: "http-loop-denied-" + denied.name, body: cases[0].body, auth: denied.auth, origin: denied.origin, status: denied.status})
		loopProblem(t, response, denied.status, denied.code)
		counts(0, 0, 0)
	}
	subjectBody := append([]byte(`{"subject":"another-user",`), cases[0].body[1:]...)
	loopProblem(t, request(1, "POST", "/api/v1/experiments/submissions", "http-loop-subject-key", subjectBody, 400), 400, "INVALID_SUBMISSION")
	counts(0, 0, 0)
	loopProblem(t, request(1, "GET", "/api/v1/experiments/submission-receipt", cases[0].key, nil, 404), 404, "NOT_FOUND")
	stable := func(got, initial submission.Receipt) {
		t.Helper()
		if got.ID != initial.ID || got.MaterialVersion != initial.MaterialVersion || got.Language != initial.Language || !got.CreatedAt.Equal(initial.CreatedAt) {
			t.Fatal("HTTP receipt identity or pinned metadata changed")
		}
	}
	accept := func(i int, tc *judgeCase) {
		t.Helper()
		created := request(i, "POST", "/api/v1/experiments/submissions", tc.key, tc.body, 202)
		tc.queued = loopFixtureReceipt(t, created, version)
		if tc.queued.State != "queued" || tc.queued.Verdict != nil || created.location != "/api/v1/experiments/submissions/"+tc.queued.ID {
			t.Fatal("HTTP acceptance did not return queued without verdict")
		}
	}
	for i := range cases {
		tc := &cases[i]
		accept(i, tc)
		for _, response := range []loopResponse{
			request(1-i, "POST", "/api/v1/experiments/submissions", tc.key, tc.body, 200),
			request(1-i, "GET", "/api/v1/experiments/submission-receipt", tc.key, nil, 200),
			request(i, "GET", "/api/v1/experiments/submissions/"+tc.queued.ID, tc.key, nil, 200),
		} {
			got := loopFixtureReceipt(t, response, version)
			stable(got, tc.queued)
			if got.State != "queued" || got.Verdict != nil {
				t.Fatal("HTTP receipt fabricated execution before Worker start")
			}
		}
	}
	loopProblem(t, request(1, "POST", "/api/v1/experiments/submissions", cases[0].key, cases[1].body, 409), 409, "IDEMPOTENCY_CONFLICT")
	counts(2, 2, 0)
	// With no Worker running, both processes must see a full queue throughout
	// the refusal and original-key checks.
	recovery := cases[0]
	recovery.key = "http-loop-overload-key-0003"
	for i := range apis {
		loopProblem(t, request(i, "POST", "/api/v1/experiments/submissions", recovery.key, recovery.body, 429), 429, "BACKLOG_LIMIT")
		loopProblem(t, request(1-i, "GET", "/api/v1/experiments/submission-receipt", recovery.key, nil, 404), 404, "NOT_FOUND")
		got := loopFixtureReceipt(t, request(i, "POST", "/api/v1/experiments/submissions", cases[1-i].key, cases[1-i].body, 200), version)
		stable(got, cases[1-i].queued)
		if got.State != "queued" || got.Verdict != nil {
			t.Fatal("accepted receipt unavailable while queue is full")
		}
		counts(2, 2, 0)
	}
	t.Log("both APIs refused the new key with 429; no receipt, task or attempt was created; accepted keys replayed")
	for _, call := range []loopRequest{
		{method: "GET", path: "/api/v1/experiments/submissions/" + cases[0].queued.ID, key: cases[0].key, status: 401},
		{method: "GET", path: "/api/v1/experiments/submission-receipt", key: cases[0].key, auth: "Bearer " + token, origin: "https://example.invalid", status: 403},
	} {
		code := "UNAUTHENTICATED"
		if call.status == 403 {
			code = "EXPERIMENT_ONLY"
		}
		loopProblem(t, sendLoopHTTP(t, ctx, client, apis[1], call), call.status, code)
	}
	ca := testpki.New(t)
	cert, certFile, keyFile := ca.Issue(t, "spiffe://onlinejudge/worker/http-loop-node/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	controller := controllerForFixture(t, ca, cert, root, version)
	if version == experiment.UbuntuVersion {
		desktop, err := workerclient.NewAcquirer(controller, ca.Roots, cert)
		if err != nil {
			t.Fatal("cannot prepare controlled mismatched Worker")
		}
		_, claimErr := desktop.Claim(ctx)
		desktop.Close()
		if !errors.Is(claimErr, workerclient.ErrMaterials) {
			t.Fatal("Ubuntu controller did not refuse a Desktop claim")
		}
		counts(2, 2, 0)
	}
	lifetime := httpLoopLifetime(t, version)
	workerEnv := httpLoopEnvironment("OJ_SANDBOX_EXPERIMENT=1", "OJ_WORKER_M0_SLOT=primary",
		"OJ_WORKER_CONTROLLER_URL="+controller, "OJ_WORKER_CA_FILE="+ca.CAFile, "OJ_WORKER_CERT_FILE="+certFile, "OJ_WORKER_KEY_FILE="+keyFile, option)
	if version == experiment.UbuntuVersion {
		workerEnv = append(workerEnv, "DOCKER_HOST=unix:///nonexistent-oj-docker.sock", "DOCKER_CONTEXT=nonexistent-oj-context",
			"DOCKER_CONFIG=/nonexistent-oj-config", "PATH=/nonexistent-oj-path")
	}
	start := time.Now()
	w := startDualProcess(t, "worker", workerEnv)
	awaitTerminal := func(tc judgeCase) {
		t.Helper()
		finished := false
		for ctx.Err() == nil {
			w.alive(t)
			got := loopFixtureReceipt(t, request(1, "GET", "/api/v1/experiments/submissions/"+tc.queued.ID, tc.key, nil, 200), version)
			stable(got, tc.queued)
			if got.State == "finished" {
				if got.Verdict == nil || *got.Verdict != tc.verdict {
					t.Fatal("unexpected HTTP final verdict")
				}
				finished = true
				break
			}
			if (got.State != "queued" && got.State != "running") || got.Verdict != nil {
				t.Fatal("invalid HTTP pending state")
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !finished {
			t.Fatal("HTTP terminal observation timed out")
		}
		for _, response := range []loopResponse{
			request(0, "GET", "/api/v1/experiments/submission-receipt", tc.key, nil, 200),
			request(0, "POST", "/api/v1/experiments/submissions", tc.key, tc.body, 200),
		} {
			got := loopFixtureReceipt(t, response, version)
			stable(got, tc.queued)
			if got.State != "finished" || got.Verdict == nil || *got.Verdict != tc.verdict {
				t.Fatal("original-key terminal receipt changed")
			}
		}
		changed, err := json.Marshal(submission.Request{MaterialVersion: version, Language: "gnu++17", Source: "changed"})
		if err != nil {
			t.Fatal("cannot encode controlled conflict")
		}
		loopProblem(t, request(0, "POST", "/api/v1/experiments/submissions", tc.key, changed, 409), 409, "IDEMPOTENCY_CONFLICT")
		var source, storedVersion, digest, node, state string
		var attempt, judge, recovery int
		if err := conn.QueryRowContext(ctx, `SELECT s.source_code,s.material_version,s.request_sha256,a.node_id,a.state,a.attempt_number,a.judge_generation,a.recovery_generation FROM submissions s JOIN judge_attempts a ON a.submission_id=s.id WHERE s.id=$1 AND s.subject_id=$2 AND s.context_id=$3`, tc.queued.ID, "m0-operator", "m0-training").Scan(&source, &storedVersion, &digest, &node, &state, &attempt, &judge, &recovery); err != nil || source != tc.source || storedVersion != version || digest != submission.Digest(submission.Request{MaterialVersion: version, Language: "gnu++17", Source: tc.source}) || node != "http-loop-node" || state != "completed" || attempt != 1 || judge != 1 || recovery != 1 {
			t.Fatal("HTTP source, fixture attribution or single real attempt not preserved")
		}
		hash := sha256.Sum256([]byte(source))
		t.Logf("HTTP queued→%s/finished; immutable source SHA256=%x; one authenticated Worker attempt", tc.verdict, hash)
	}
	for _, tc := range cases {
		awaitTerminal(tc)
	}
	counts(2, 2, 2)
	loopProblem(t, request(0, "GET", "/api/v1/experiments/submission-receipt", recovery.key, nil, 404), 404, "NOT_FOUND")
	accept(1, &recovery)
	if recovery.queued.ID == cases[0].queued.ID || recovery.queued.ID == cases[1].queued.ID {
		t.Fatal("retry after capacity recovery did not create a distinct submission")
	}
	t.Logf("same refused key/body accepted after both original submissions finished; worker start to acceptance=%s", time.Since(start).Round(time.Millisecond))
	awaitTerminal(recovery)
	counts(3, 3, 3)
	lifetime(t, w)
	for _, api := range apis {
		api.process.stop(t)
	}
	t.Log("two real API processes → mTLS controller → real Worker → HTTP AC/WA/AC; backlog recovery and normal process drain passed; no browser entry enabled")
}

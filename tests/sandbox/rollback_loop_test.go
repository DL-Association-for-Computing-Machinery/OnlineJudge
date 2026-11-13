//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

const (
	rollbackNode    = "rollback-loop-node"
	rollbackVersion = experiment.PinnedVersion
	rollbackCurrent = "../../.local/bin"
)

type rollbackFixture struct {
	database               restoreLoopDatabase
	root, tokenFile, token string
	ca                     *testpki.Authority
	cert                   tls.Certificate
	certFile, keyFile      string
	client                 *http.Client
	stopWorker             func(*testing.T, *dualProcess)
}

type rollbackCase struct {
	key, source, verdict string
	body                 []byte
	accepted             submission.Receipt
}

func TestCompatibleRollbackLoop(t *testing.T) {
	if os.Getenv("OJ_SANDBOX_ROLLBACK") != "1" {
		t.Skip("run python3 scripts/test-worker.py rollback-loop")
	}
	verifyEnvironment(t)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	oldDir := os.Getenv("OJ_ROLLBACK_BIN")
	if !filepath.IsAbs(oldDir) {
		t.Fatal("separately built prior release is required")
	}
	oldHash, currentHash := rollbackBinaryHash(t, oldDir), rollbackBinaryHash(t, rollbackCurrent)
	if oldHash == currentHash {
		t.Fatal("the two Worker executables must be different")
	}
	t.Logf("prior revision %s, Worker SHA-256 old=%x current=%x", os.Getenv("OJ_ROLLBACK_REVISION"), oldHash, currentHash)
	f := newRollbackFixture(t, ctx, oldDir)
	slot := dualDocker(t, "inspect", "--format", "{{.Id}}", container)
	var migrationRows, schema int
	if err := f.database.pool.QueryRow(ctx, `SELECT count(*), max(version_id) FROM goose_db_version`).Scan(&migrationRows, &schema); err != nil || schema != 20 {
		t.Fatal("expected the fixed compatible schema before switching releases")
	}

	oldAPI, oldController, oldWorker := f.start(t, ctx, oldDir)
	history := f.accept(t, ctx, oldAPI, "rollback-history-0001", "AC", "// 历史\r\n"+rollbackSum)
	f.await(t, ctx, oldAPI, oldWorker, history)
	oldResult := f.result(t, ctx, history)
	oldAPI.process.stop(t)
	f.stopWorker(t, oldWorker)
	oldController.stop(t)
	if t.Failed() {
		t.FailNow()
	}

	// Upgrade while the old processes are stopped, on the same database.
	runFixtureMigration(t, ctx, rollbackCurrent, f.database.dsn, f.root, rollbackVersion, []string{"up"})
	restartDrainedSandbox(t, slot)
	verifyEnvironment(t)
	newAPI, newController, newWorker := f.start(t, ctx, rollbackCurrent)
	completed := f.accept(t, ctx, newAPI, "rollback-current-0002", "WA", "#include <cstdio>\nint main(){std::puts(\"wrong\");}")
	f.await(t, ctx, newAPI, newWorker, completed)
	currentResult := f.result(t, ctx, completed)
	f.stopWorker(t, newWorker)
	queued := f.accept(t, ctx, newAPI, "rollback-current-0003", "AC", "// 切换后处理\r\n"+rollbackSum)
	newAPI.process.stop(t)
	newController.stop(t)
	if t.Failed() {
		t.FailNow()
	}
	f.counts(t, ctx, 2, migrationRows+2, 22)

	start := time.Now()
	// Down22 drops only the rate gate. Down21 requires an empty email ledger.
	// Neither step may change accepted judging work.
	runFixtureMigration(t, ctx, rollbackCurrent, f.database.dsn, f.root, rollbackVersion, []string{"down"})
	runFixtureMigration(t, ctx, rollbackCurrent, f.database.dsn, f.root, rollbackVersion, []string{"down"})
	restartDrainedSandbox(t, slot)
	verifyEnvironment(t)
	reverted := startHTTPLoopAPIFrom(t, ctx, oldDir, f.client, f.database.dsn, f.tokenFile)
	for _, tc := range []rollbackCase{history, completed} {
		f.checkReceipt(t, ctx, reverted, tc, "finished", http.MethodGet)
	}
	f.checkReceipt(t, ctx, reverted, queued, "queued", http.MethodGet)
	// Replay before the old Worker starts. A second POST may only read back the
	// accepted ID, never create a replacement submission or another attempt.
	f.checkReceipt(t, ctx, reverted, queued, "queued", http.MethodPost)
	f.counts(t, ctx, 2, migrationRows, 20)
	controller, worker := f.startWorker(t, oldDir)
	f.await(t, ctx, reverted, worker, queued)
	f.checkReceipt(t, ctx, reverted, queued, "finished", http.MethodPost)
	if f.result(t, ctx, history) != oldResult || f.result(t, ctx, completed) != currentResult {
		t.Fatal("reverting the application changed a completed attempt")
	}
	f.result(t, ctx, queued)
	f.counts(t, ctx, 3, migrationRows, 20)
	reverted.process.stop(t)
	f.stopWorker(t, worker)
	controller.stop(t)
	t.Logf("same database after empty-token downgrade: old AC and new WA unchanged; prior release finished the queued new submission as AC; 3 submissions/3 attempts; revert and drain=%s", time.Since(start))
}

const rollbackSum = "#include <cstdio>\nint main(){int a,b;if(std::scanf(\"%d%d\",&a,&b)!=2)return 1;std::printf(\"%d\\n\",a+b);}\n"

func rollbackBinaryHash(t *testing.T, directory string) [32]byte {
	t.Helper()
	file, err := os.Open(filepath.Join(directory, "worker"))
	if err != nil {
		t.Fatal("cannot open the test Worker")
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatal("cannot hash the test Worker")
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result
}

func newRollbackFixture(t *testing.T, ctx context.Context, directory string) rollbackFixture {
	t.Helper()
	database := openDesktopTestDatabase(t, ctx, os.Getenv("OJ_ROLLBACK_CONTAINER"), os.Getenv("OJ_ROLLBACK_DATABASE"))
	root := t.TempDir()
	runFixtureMigration(t, ctx, directory, database.dsn, root, rollbackVersion, []string{"up"}, []string{"fixture"}, []string{"backlog", "2", "2"})
	if _, err := database.pool.Exec(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)`, rollbackNode); err != nil {
		t.Fatal("cannot provision the synthetic rollback Worker")
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		t.Fatal("cannot prepare the private fixture credential")
	}
	token, tokenFile := hex.EncodeToString(secret[:]), filepath.Join(t.TempDir(), "experiment-token")
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		t.Fatal("cannot store the private fixture credential")
	}
	ca := testpki.New(t)
	cert, certFile, keyFile := ca.Issue(t, "spiffe://onlinejudge/worker/"+rollbackNode+"/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	transport := &http.Transport{MaxConnsPerHost: 2}
	t.Cleanup(transport.CloseIdleConnections)
	return rollbackFixture{database: database, root: root, token: token, tokenFile: tokenFile, ca: ca, cert: cert,
		certFile: certFile, keyFile: keyFile, client: &http.Client{Transport: transport, Timeout: 2 * time.Second},
		stopWorker: httpLoopLifetime(t, rollbackVersion)}
}

func (f rollbackFixture) start(t *testing.T, ctx context.Context, directory string) (loopAPI, *dualProcess, *dualProcess) {
	t.Helper()
	api := startHTTPLoopAPIFrom(t, ctx, directory, f.client, f.database.dsn, f.tokenFile)
	controller, worker := f.startWorker(t, directory)
	return api, controller, worker
}

func (f rollbackFixture) startWorker(t *testing.T, directory string) (*dualProcess, *dualProcess) {
	t.Helper()
	address, controller := controllerFrom(t, directory, f.ca, f.cert, f.root, rollbackVersion, f.database.dsn)
	worker := startDualProcessFrom(t, directory, "worker", httpLoopEnvironment("OJ_SANDBOX_EXPERIMENT=1", "OJ_WORKER_M0_SLOT=primary",
		"OJ_WORKER_CONTROLLER_URL="+address, "OJ_WORKER_CA_FILE="+f.ca.CAFile, "OJ_WORKER_CERT_FILE="+f.certFile, "OJ_WORKER_KEY_FILE="+f.keyFile))
	return controller, worker
}

func (f rollbackFixture) accept(t *testing.T, ctx context.Context, api loopAPI, key, verdict, source string) rollbackCase {
	t.Helper()
	body, err := json.Marshal(submission.Request{Source: source, MaterialVersion: rollbackVersion, Language: "gnu++17"})
	if err != nil {
		t.Fatal(err)
	}
	response := sendLoopHTTP(t, ctx, f.client, api, loopRequest{method: http.MethodPost, path: "/api/v1/experiments/submissions",
		key: key, auth: "Bearer " + f.token, body: body, status: http.StatusAccepted})
	accepted := loopFixtureReceipt(t, response, rollbackVersion)
	if accepted.State != "queued" || accepted.Verdict != nil || response.location != "/api/v1/experiments/submissions/"+accepted.ID {
		t.Fatal("release failed to accept the submission with its original receipt")
	}
	return rollbackCase{key: key, source: source, verdict: verdict, body: body, accepted: accepted}
}

func (f rollbackFixture) receipt(t *testing.T, ctx context.Context, api loopAPI, tc rollbackCase, method string) submission.Receipt {
	t.Helper()
	call := loopRequest{method: method, path: "/api/v1/experiments/submission-receipt", key: tc.key, auth: "Bearer " + f.token, status: http.StatusOK}
	if method == http.MethodPost {
		call.path, call.body = "/api/v1/experiments/submissions", tc.body
	}
	got := loopFixtureReceipt(t, sendLoopHTTP(t, ctx, f.client, api, call), rollbackVersion)
	if got.ID != tc.accepted.ID || !got.CreatedAt.Equal(tc.accepted.CreatedAt) {
		t.Fatal("application switch changed the submission identity")
	}
	return got
}

func (f rollbackFixture) checkReceipt(t *testing.T, ctx context.Context, api loopAPI, tc rollbackCase, state, method string) {
	t.Helper()
	got := f.receipt(t, ctx, api, tc, method)
	if got.State != state || (state == "finished" && (got.Verdict == nil || *got.Verdict != tc.verdict)) || (state != "finished" && got.Verdict != nil) {
		t.Fatal("submission receipt differs from its expected state or verdict")
	}
}

func (f rollbackFixture) await(t *testing.T, ctx context.Context, api loopAPI, worker *dualProcess, tc rollbackCase) {
	t.Helper()
	for ctx.Err() == nil {
		worker.alive(t)
		got := f.receipt(t, ctx, api, tc, http.MethodGet)
		if got.State == "finished" && got.Verdict != nil && *got.Verdict == tc.verdict {
			return
		}
		if (got.State != "queued" && got.State != "running") || got.Verdict != nil {
			t.Fatal("unexpected real Worker result")
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the Worker did not finish the accepted submission")
}

func (f rollbackFixture) result(t *testing.T, ctx context.Context, tc rollbackCase) restoredAttempt {
	t.Helper()
	var source string
	var result restoredAttempt
	if err := f.database.pool.QueryRow(ctx, `SELECT s.source_code, a.result_sha256, a.result_payload::text
		FROM submissions s JOIN judge_attempts a ON a.submission_id=s.id
		WHERE s.id=$1 AND s.material_version=$2 AND a.judge_generation=1 AND a.recovery_generation=1
		AND a.attempt_number=1 AND a.node_id=$3 AND a.state='completed' AND a.result_payload->>'Verdict'=$4`,
		tc.accepted.ID, rollbackVersion, rollbackNode, tc.verdict).Scan(&source, &result.fingerprint, &result.payload); err != nil || source != tc.source {
		t.Fatal("submission source or completed attempt changed across release switch")
	}
	return result
}

func (f rollbackFixture) counts(t *testing.T, ctx context.Context, attempts, migrationRows, version int) {
	t.Helper()
	var submissions, tasks, actualAttempts, actualMigrations, schema int
	if err := f.database.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM submissions), (SELECT count(*) FROM judge_tasks),
		(SELECT count(*) FROM judge_attempts), count(*), max(version_id) FROM goose_db_version`).Scan(&submissions, &tasks, &actualAttempts, &actualMigrations, &schema); err != nil || submissions != 3 || tasks != 3 || actualAttempts != attempts || actualMigrations != migrationRows || schema != version {
		t.Fatal("release switch changed the submission, task, attempt or migration inventory")
	}
}

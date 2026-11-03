//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWorkerNetworkFaults(t *testing.T) {
	if os.Getenv("OJ_SANDBOX_NETWORK") != "1" {
		t.Skip("run python3 scripts/test-worker.py network-fault")
	}
	verifyEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := os.Getenv("OJ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("empty disposable database required")
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }() // Fixture connection only; all operations are checked.
	var tables int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatal("network-fault database must be empty")
	}
	provider, err := db.Provider(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	version, manifest, err := experiment.SelectFixture(experiment.PinnedVersion)
	if err != nil {
		t.Fatal(err)
	}
	store, err := postgres.New(pool, version, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterFixture(ctx); err != nil {
		t.Fatal(err)
	}
	configureSandboxBacklog(t, ctx, pool)
	for _, fault := range []string{"partition", "acknowledgement"} {
		if !t.Run(fault, func(t *testing.T) { testWorkerNetworkFault(t, pool, store, fault) }) {
			break // Stop after a failed exclusive-slot experiment; do not mask its state.
		}
	}
}

const networkSource = "#include <cstdio>\n#include <unistd.h>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);usleep(300000);printf(\"%d\",a+b);}\n"

type durableNetworkResult struct {
	payload []byte
	digest  string
}

func testWorkerNetworkFault(t *testing.T, pool *pgxpool.Pool, store *postgres.Store, fault string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	ids := [2]string{
		dualDocker(t, "inspect", "--format", "{{.Id}}", "oj-m0-sandbox"),
		dualDocker(t, "inspect", "--format", "{{.Id}}", "oj-m0-sandbox-secondary"),
	}
	for _, id := range ids {
		restartDrainedSandbox(t, id)
	}
	node, peer := "network-"+fault, "network-peer-"+fault
	if _, err := pool.Exec(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,1),($2,1)`, node, peer); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := experiment.WriteMaterials(root); err != nil {
		t.Fatal(err)
	}
	ca := testpki.New(t)
	cert, certFile, key := ca.Issue(t, "spiffe://onlinejudge/worker/"+node+"/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	endpoints := []string{dualController(t, ca, cert, root), dualController(t, ca, cert, root)}
	relay, err := newFaultRelay(ctx, strings.TrimPrefix(endpoints[0], "https://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !relay.Close() {
			t.Error("network relay failed to drain")
		}
	})
	accept := func(key string) string {
		t.Helper()
		row, _, err := store.Accept(ctx, node, "m0-training", "network-"+fault+"-"+key,
			submission.Request{MaterialVersion: experiment.PinnedVersion, Language: "gnu++17", Source: networkSource})
		if err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	// Reporter has its own connection. A completed warmup followed by execution
	// of the target proves its acknowledgement reached this same Worker before
	// the downstream gate closes; no TLS handshake has to pass the closed gate.
	warmup := accept("warmup")
	p := dualWorker(t, ca, certFile, key, relay.endpoint(), "primary")
	waitNetworkAC(t, ctx, store, node, warmup, p)
	target, next := accept("target"), accept("next")
	var pid, token string
	for ctx.Err() == nil {
		p.alive(t)
		if pid = dualPID(t, ids[0]); pid != "" {
			if err := pool.QueryRow(ctx, `SELECT token FROM judge_attempts WHERE submission_id=$1 AND state='leased'`, target).Scan(&token); err != nil {
				t.Fatal("observed process did not belong to the target attempt")
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == "" {
		t.Fatal("no live target before network fault")
	}
	var committed durableNetworkResult
	if fault == "acknowledgement" {
		blockNetworkAcknowledgement(t, ctx, pool, relay, target, token)
		waitNetworkAC(t, ctx, store, node, target, p)
		if err := pool.QueryRow(ctx, `SELECT result_payload,result_sha256 FROM judge_attempts WHERE submission_id=$1 AND state='completed'`, target).Scan(&committed.payload, &committed.digest); err != nil {
			t.Fatal(err)
		}
	} else {
		relay.Block(relayUp | relayDown)
	}
	waitNetworkFailure(t, p)
	dualStopped(t, ids[0], pid)
	var nextState string
	var nextAttempts int
	if err := pool.QueryRow(ctx, `SELECT state FROM judge_tasks WHERE submission_id=$1`, next).Scan(&nextState); err != nil || nextState != "queued" {
		t.Fatal("failed worker claimed the pre-existing next task")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM judge_attempts WHERE submission_id=$1`, next).Scan(&nextAttempts); err != nil || nextAttempts != 0 {
		t.Fatal("failed worker admitted another attempt")
	}
	if fault == "acknowledgement" {
		if relay.downHeld.Load() == 0 {
			t.Fatal("no real downstream bytes were blocked")
		}
	} else {
		if relay.upHeld.Load() == 0 {
			t.Fatal("no real upstream bytes were blocked")
		}
		var attempts, results int
		if err := pool.QueryRow(ctx, `SELECT count(*),count(result_payload) FROM judge_attempts WHERE submission_id=$1`, target).Scan(&attempts, &results); err != nil || attempts != 1 || results != 0 {
			t.Fatal("partitioned attempt published evidence or reran")
		}
		row, err := store.Get(ctx, node, "m0-training", target)
		if err != nil || row.Verdict != nil || row.State == "finished" {
			t.Fatal("partition fabricated a contestant verdict")
		}
	}
	// Close all pre-fault TLS streams before admitting restored connections.
	// Nothing here edits a lease timestamp or calls Store.Recover.
	relay.Resume()
	var healthy *dualProcess
	if fault == "partition" {
		_, peerCert, peerKey := ca.Issue(t, "spiffe://onlinejudge/worker/"+peer+"/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
		healthy = dualWorker(t, ca, peerCert, peerKey, endpoints[1], "secondary")
	} else {
		restartDrainedSandbox(t, ids[0])
		healthy = dualWorker(t, ca, certFile, key, relay.endpoint(), "primary")
	}
	waitNetworkAC(t, ctx, store, node, target, healthy)
	waitNetworkAC(t, ctx, store, node, next, healthy)
	checkNetworkEvidence(t, ctx, pool, ca, cert, endpoints, fault, target, token, peer, committed)
	healthy.stop(t)
	if fault == "partition" {
		dualStopped(t, ids[1], "")
	} else {
		dualStopped(t, ids[0], "")
	}
	t.Logf("%s: opaque mTLS traffic blocked (up=%d down=%d bytes); failed worker stopped/PID absent with next task untouched; restored execution and both-controller evidence checks passed", fault, relay.upHeld.Load(), relay.downHeld.Load())
}

func blockNetworkAcknowledgement(t *testing.T, ctx context.Context, pool *pgxpool.Pool, relay *faultRelay, target, token string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), time.Second)
		defer done()
		if err := tx.Rollback(cleanup); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error("network fault coordination transaction did not close")
		}
	}()
	var state, heldToken string
	var payload []byte
	// This short fixture lock orders the gate before the real report commit.
	// It prevents mistaking a later failed Claim for a lost report reply. No
	// task/lease/result values change, and only the real controller commits it.
	if err := tx.QueryRow(ctx, `SELECT token,state,result_payload FROM judge_attempts WHERE submission_id=$1 FOR UPDATE`, target).Scan(&heldToken, &state, &payload); err != nil || state != "leased" || payload != nil || heldToken != token {
		t.Fatal("target report committed before the fault barrier")
	}
	relay.Block(relayDown)
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal("cannot release network fault coordination lock")
	}
}

func waitNetworkAC(t *testing.T, ctx context.Context, store *postgres.Store, subject, id string, p *dualProcess) {
	t.Helper()
	for ctx.Err() == nil {
		row, err := store.Get(ctx, subject, "m0-training", id)
		if err != nil {
			t.Fatal(err)
		}
		if row.State == "finished" {
			if row.Verdict == nil || *row.Verdict != "AC" {
				t.Fatal("network fixture did not finish AC")
			}
			return
		}
		p.alive(t)
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("network recovery result timeout")
}

func waitNetworkFailure(t *testing.T, p *dualProcess) {
	t.Helper()
	select {
	case err := <-p.done:
		p.stopped = true
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			t.Fatal("network fault did not cause a failed worker exit")
		}
	case <-time.After(25 * time.Second):
		t.Fatal("network fault did not stop worker within the experiment observation budget")
	}
}

func checkNetworkEvidence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ca *testpki.Authority, cert tls.Certificate, endpoints []string, fault, target, oldToken, peer string, committed durableNetworkResult) {
	t.Helper()
	var payload []byte
	var digest, owner string
	var attempt, judgeGeneration, recoveryGeneration, attempts int
	if err := pool.QueryRow(ctx, `SELECT result_payload,result_sha256,node_id,attempt_number,judge_generation,recovery_generation FROM judge_attempts WHERE submission_id=$1 AND state='completed'`, target).Scan(&payload, &digest, &owner, &attempt, &judgeGeneration, &recoveryGeneration); err != nil {
		t.Fatal(err)
	}
	wantAttempt := 1
	if fault == "acknowledgement" && (string(payload) != string(committed.payload) || digest != committed.digest) {
		t.Fatal("recovery changed the originally committed unacknowledged result")
	}
	if fault == "partition" {
		wantAttempt = 2
		var oldState string
		if err := pool.QueryRow(ctx, `SELECT state FROM judge_attempts WHERE token=$1`, oldToken).Scan(&oldState); err != nil || oldState != "expired" || owner != peer {
			t.Fatal("peer did not recover the naturally expired attempt")
		}
	}
	if attempt != wantAttempt || judgeGeneration != 1 || recoveryGeneration != 1 {
		t.Fatal("network fault changed generations or duplicated execution")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM judge_attempts WHERE submission_id=$1`, target).Scan(&attempts); err != nil || attempts != wantAttempt {
		t.Fatal("network fault produced extra attempts")
	}
	var result submission.Result
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "AC" {
		t.Fatal("durable payload did not retain the real AC result")
	}
	for _, endpoint := range endpoints {
		r, err := workerclient.NewReporter(endpoint, ca.Roots, cert)
		if err != nil {
			t.Fatal(err)
		}
		err = r.Report(ctx, oldToken, result)
		if fault == "partition" {
			// This is a late authorization probe using the peer's real result,
			// not a claim that the disconnected worker froze that result.
			if !errors.Is(err, submission.ErrLease) {
				t.Fatal("late partitioned attempt was accepted")
			}
		} else {
			if err != nil {
				t.Fatal("committed unacknowledged result was not confirmed consistently")
			}
			conflict := result
			conflict.Verdict = "WA"
			if err := r.Report(ctx, oldToken, conflict); !errors.Is(err, submission.ErrResultConflict) {
				t.Fatal("conflicting replay was not rejected")
			}
		}
		r.Close()
	}
	var unchanged []byte
	var sameDigest string
	if err := pool.QueryRow(ctx, `SELECT result_payload,result_sha256 FROM judge_attempts WHERE submission_id=$1 AND state='completed'`, target).Scan(&unchanged, &sameDigest); err != nil || string(unchanged) != string(payload) || sameDigest != digest {
		t.Fatal("replays changed durable result evidence")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM judge_attempts WHERE submission_id=$1`, target).Scan(&attempts); err != nil || attempts != wantAttempt {
		t.Fatal("replays created extra attempts")
	}
}

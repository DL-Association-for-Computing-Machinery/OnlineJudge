//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Real command processes and an empty disposable database exercise rejection,
// confirmed shutdown and automatic recovery. Only fixture data is corrupted.
func TestWorkerFaults(t *testing.T) {
	if os.Getenv("OJ_SANDBOX_FAULT") != "1" {
		t.Skip("run python3 scripts/test-worker.py worker-fault with the dedicated primary slot")
	}
	verifyEnvironment(t)
	dsn := os.Getenv("OJ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("disposable database required")
	}
	ctx := context.Background()
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Cleanup does not change the completed operation or its primary error.
	defer func() { _ = conn.Close() }()
	var tables int
	if err := conn.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatal("fault database must be empty")
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
	for _, fault := range []string{"disabled", "rotated", "missing", "corrupt"} {
		if !t.Run(fault, func(t *testing.T) { testWorkerFault(t, pool, store, fault) }) {
			break // A failed exclusive-slot experiment requires operator inspection.
		}
	}
}

func testWorkerFault(t *testing.T, pool *pgxpool.Pool, store *postgres.Store, fault string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	id := dualDocker(t, "inspect", "--format", "{{.Id}}", container)
	restartFaultSandbox(t, ctx, id)
	node, replacement := "fault-"+fault, "replacement-"+fault
	if _, err := pool.Exec(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,1),($2,1)`, node, replacement); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := experiment.WriteMaterials(root); err != nil {
		t.Fatal(err)
	}
	ca := testpki.New(t)
	oldCert, certFile, key := ca.Issue(t, "spiffe://onlinejudge/worker/"+node+"/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	endpoints := []string{dualController(t, ca, oldCert, root), dualController(t, ca, oldCert, root)}
	// Damage the last answer: the downloader must discard the preceding three
	// valid blobs. Files belong solely to this subtest's temporary material root.
	material := filepath.Join(root, hashBytes([]byte("-3\n")))
	switch fault {
	case "missing":
		if err := os.Remove(material); err != nil {
			t.Fatal(err)
		}
	case "corrupt":
		if err := os.WriteFile(material, []byte("999\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	const source = "#include <cstdio>\n#include <unistd.h>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);sleep(1);printf(\"%d\",a+b);}\n"
	accept := func(key string) string {
		t.Helper()
		row, _, err := store.Accept(ctx, "fault-fixture", "m0-training", "worker-fault-"+fault+"-"+key,
			submission.Request{MaterialVersion: experiment.PinnedVersion, Language: "gnu++17", Source: source})
		if err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	rowID := accept("broken")
	// Both tasks exist before startup, so refusing the next claim cannot pass
	// merely because that task arrived after the failed process already exited.
	nextID := accept("next")
	p := dualWorker(t, ca, certFile, key, endpoints[0], "primary")
	var token, pid string
	if fault == "disabled" || fault == "rotated" {
		for ctx.Err() == nil {
			p.alive(t)
			if pid = dualPID(t, id); pid != "" {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if pid == "" {
			t.Fatal("no live contestant before revocation")
		}
		var err error
		if fault == "disabled" {
			_, err = pool.Exec(ctx, `UPDATE worker_nodes SET enabled=false WHERE id=$1`, node)
		} else {
			_, err = pool.Exec(ctx, `UPDATE worker_nodes SET credential_version=2 WHERE id=$1`, node)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-p.done:
		p.stopped = true
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			t.Fatalf("fault must cause operation failure, got %v", err)
		}
	case <-ctx.Done():
		t.Fatal("faulty worker failed to exit")
	}
	dualStopped(t, id, pid)
	var attempts, results int
	if err := pool.QueryRow(ctx, `SELECT count(*),count(result_payload) FROM judge_attempts WHERE submission_id=$1`, rowID).Scan(&attempts, &results); err != nil || attempts != 1 || results != 0 {
		t.Fatal("faulty attempt published evidence or retried")
	}
	if err := pool.QueryRow(ctx, `SELECT token FROM judge_attempts WHERE submission_id=$1`, rowID).Scan(&token); err != nil {
		t.Fatal(err)
	}
	row, err := store.Get(ctx, "fault-fixture", "m0-training", rowID)
	if err != nil || row.Verdict != nil || row.State == "finished" {
		t.Fatal("fault published a contestant verdict")
	}
	row, err = store.Get(ctx, "fault-fixture", "m0-training", nextID)
	if err != nil || row.State != "queued" {
		t.Fatal("faulty worker claimed the next task")
	}
	if fault == "missing" || fault == "corrupt" {
		if err := os.WriteFile(material, []byte("-3\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Recovery restarts the host explicitly after confirmed stop; the controller
	// alone expires/requeues attempts. No timestamp edits or manual Recover calls.
	restartFaultSandbox(t, ctx, id)
	identity := replacement + "/1"
	if fault == "rotated" {
		identity = node + "/2"
		replacement = node
	}
	_, newCert, newKey := ca.Issue(t, "spiffe://onlinejudge/worker/"+identity, x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	healthy := dualWorker(t, ca, newCert, newKey, endpoints[1], "primary")
	for _, wanted := range []string{rowID, nextID} {
		for {
			healthy.alive(t)
			row, err := store.Get(ctx, "fault-fixture", "m0-training", wanted)
			if err != nil {
				t.Fatal(err)
			}
			if row.State == "finished" {
				if row.Verdict == nil || *row.Verdict != "AC" {
					t.Fatal("recovered source did not produce AC")
				}
				break
			}
			if ctx.Err() != nil {
				t.Fatal("automatic recovery timeout")
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	var payload []byte
	var owner, oldState string
	if err := pool.QueryRow(ctx, `SELECT node_id,result_payload FROM judge_attempts WHERE submission_id=$1 AND state='completed' AND attempt_number=2 AND judge_generation=1 AND recovery_generation=1`, rowID).Scan(&owner, &payload); err != nil || owner != replacement {
		t.Fatal("replacement did not complete attempt 2 in the original generations")
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM judge_attempts WHERE token=$1`, token).Scan(&oldState); err != nil || oldState != "expired" {
		t.Fatal("old attempt not expired")
	}
	var result submission.Result
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	// A new real result with the old credential/token is an authorization probe,
	// not a claim that the failed worker froze or retained any result.
	want := submission.ErrLease
	if fault == "disabled" || fault == "rotated" {
		want = submission.ErrNode
	}
	for _, endpoint := range endpoints {
		r, err := workerclient.NewReporter(endpoint, ca.Roots, oldCert)
		if err != nil {
			t.Fatal(err)
		}
		err = r.Report(ctx, token, result)
		r.Close()
		if !errors.Is(err, want) {
			t.Fatalf("late report rejection: %v", err)
		}
	}
	var unchanged []byte
	if err := pool.QueryRow(ctx, `SELECT result_payload FROM judge_attempts WHERE submission_id=$1 AND state='completed'`, rowID).Scan(&unchanged); err != nil || string(unchanged) != string(payload) {
		t.Fatal("late report modified recovered evidence")
	}
	healthy.stop(t)
	dualStopped(t, id, "")
	t.Log("fault caused nonzero exit, confirmed sandbox stop and no result/next claim; automatic recovery and explicit host restart produced AC; both controllers rejected stale report")
}

func restartFaultSandbox(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	dualDocker(t, "start", id)
	for ctx.Err() == nil {
		cmd := exec.CommandContext(ctx, "docker", "--context", "desktop-linux", "exec", id, "curl", "-fsS", "--max-time", "1", "http://127.0.0.1:5050/version")
		if cmd.Run() == nil {
			verifyEnvironment(t)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("sandbox restart timeout")
}

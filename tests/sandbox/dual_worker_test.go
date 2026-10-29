//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

// Uses two real controller processes and two real workers. The test driver alone
// owns the disposable database and provisions submissions; workers receive no DSN.
// SIGTERM exercises confirmed termination, not SIGKILL or physical host failure.
func TestDualWorkerRecovery(t *testing.T) {
	if os.Getenv("OJ_SANDBOX_DUAL") != "1" {
		t.Skip("run python3 scripts/test-worker.py dual-worker with both dedicated slots")
	}
	if os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" {
		t.Fatal("explicit sandbox opt-in required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	dsn := os.Getenv("OJ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("disposable database required")
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Cleanup does not change the completed operation or its primary error.
	defer func() { _ = conn.Close() }()
	var tables int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatal("dual-worker database must be empty")
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
	if _, err := pool.Exec(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES('dual-primary',1),('dual-secondary',1)`); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := experiment.WriteMaterials(root); err != nil {
		t.Fatal(err)
	}
	ca := testpki.New(t)
	var certs [2]tls.Certificate
	var certFiles, keys, ids [2]string
	slots := [2]string{"primary", "secondary"}
	for i, name := range slots {
		certs[i], certFiles[i], keys[i] = ca.Issue(t, "spiffe://onlinejudge/worker/dual-"+name+"/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
		slot, _ := workerhost.ParseM0Slot(name)
		ids[i] = dualDocker(t, "inspect", "--format", "{{.Id}}", slot.ContainerName())
		host, err := workerhost.OpenM0Slot(ctx, slot)
		if err != nil {
			t.Fatal(err)
		}
		if err := execution.NewGoJudge(host, host.Verify).Verify(ctx, experiment.PinnedEnvironment()); err != nil {
			t.Fatal(err)
		}
	}
	if ids[0] == ids[1] {
		t.Fatal("workers must own distinct containers")
	}
	endpoints := [2]string{dualController(t, ca, certs[0], root), dualController(t, ca, certs[1], root)}
	workers := [2]*dualProcess{}
	for i := range workers {
		workers[i] = dualWorker(t, ca, certFiles[i], keys[i], endpoints[i], slots[i])
	}
	accept := func(key, source string) string {
		t.Helper()
		row, _, err := store.Accept(ctx, "dual-fixture", "m0-training", key, submission.Request{MaterialVersion: version, Language: "gnu++17", Source: source})
		if err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	waitResult := func(id, verdict string) {
		t.Helper()
		for ctx.Err() == nil {
			row, err := store.Get(ctx, "dual-fixture", "m0-training", id)
			if err != nil {
				t.Fatal(err)
			}
			if row.State == "finished" {
				if row.Verdict == nil || *row.Verdict != verdict {
					t.Fatal("unexpected durable verdict")
				}
				return
			}
			for _, p := range workers {
				if !p.stopped {
					p.alive(t)
				}
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatal("durable result timeout")
	}
	const ac = "#include <cstdio>\n#include <unistd.h>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);usleep(300000);printf(\"%d\",a+b);}\n"
	var submissions []string
	for i := range 8 {
		submissions = append(submissions, accept(fmt.Sprintf("dual-concurrent-%d", i), ac))
	}
	// Require overlapping live leases, not just two nodes observed sequentially.
	for deadline := time.Now().Add(10 * time.Second); ; {
		var active int
		if err := pool.QueryRow(ctx, `SELECT count(DISTINCT node_id) FROM judge_attempts WHERE state='leased' AND lease_until>clock_timestamp()`).Scan(&active); err != nil {
			t.Fatal(err)
		}
		if active == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("two workers never held overlapping live leases")
		}
		for _, p := range workers {
			p.alive(t)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, id := range submissions {
		waitResult(id, "AC")
	}
	var attempts, nodes int
	if err := pool.QueryRow(ctx, `SELECT count(*),count(DISTINCT node_id) FROM judge_attempts`).Scan(&attempts, &nodes); err != nil || attempts != 8 || nodes != 2 {
		t.Fatal("both workers must complete distinct first attempts")
	}
	// A live contestant gives a bounded window to stop its owning worker. Do not
	// modify lease timestamps or call Recover: the controllers must do real expiry.
	id := accept("dual-interrupted", "#include <cstdio>\n#include <unistd.h>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);sleep(1);printf(\"%d\",a+b);}\n")
	var token, owner, pid string
	for ctx.Err() == nil {
		err := pool.QueryRow(ctx, `SELECT token,node_id FROM judge_attempts WHERE submission_id=$1 AND state='leased'`, id).Scan(&token, &owner)
		if err == nil {
			i := 0
			if owner == "dual-secondary" {
				i = 1
			}
			pid = dualPID(t, ids[i])
			if pid != "" {
				break
			}
		}
		for _, p := range workers {
			p.alive(t)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid == "" {
		t.Fatal("live contestant was not observed")
	}
	failed := 0
	if owner == "dual-secondary" {
		failed = 1
	}
	survivor := 1 - failed
	workers[failed].stop(t)
	dualStopped(t, ids[failed], pid)
	if dualDocker(t, "inspect", "--format", "{{.State.Running}}", ids[survivor]) != "true" {
		t.Fatal("stopping one worker stopped its peer")
	}
	row, err := store.Get(ctx, "dual-fixture", "m0-training", id)
	if err != nil || row.Verdict != nil || row.State == "finished" {
		t.Fatal("interrupted attempt published a verdict")
	}
	waitResult(id, "AC")
	var completedBy, oldState string
	var payload []byte
	if err := pool.QueryRow(ctx, `SELECT node_id,result_payload FROM judge_attempts WHERE submission_id=$1 AND state='completed' AND judge_generation=1 AND recovery_generation=1 AND attempt_number=2`, id).Scan(&completedBy, &payload); err != nil || completedBy != "dual-"+slots[survivor] {
		t.Fatal("peer did not complete the second attempt in the same generations")
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM judge_attempts WHERE token=$1`, token).Scan(&oldState); err != nil || oldState != "expired" {
		t.Fatal("old attempt not expired")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM judge_attempts WHERE submission_id=$1`, id).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatal("unexpected recovery attempts")
	}
	// Deliberately replay the peer's real result with the old token: this is an
	// authorization probe, not evidence that the interrupted worker froze a result.
	var result submission.Result
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	reporter, err := workerclient.NewReporter(endpoints[survivor], ca.Roots, certs[failed])
	if err != nil {
		t.Fatal(err)
	}
	defer reporter.Close()
	if err := reporter.Report(ctx, token, result); err != submission.ErrLease {
		t.Fatal("old attempt report was not rejected")
	}
	var unchanged []byte
	if err := pool.QueryRow(ctx, `SELECT result_payload FROM judge_attempts WHERE submission_id=$1 AND state='completed'`, id).Scan(&unchanged); err != nil || string(unchanged) != string(payload) {
		t.Fatal("late report modified recovered evidence")
	}
	waitResult(accept("dual-peer-continues", ac), "AC")
	workers[survivor].stop(t)
	dualStopped(t, ids[survivor], "")
	t.Log("two controller and worker processes: eight exclusive real AC results, live SIGTERM/PID stop, natural lease expiry and peer AC on attempt 2, late report rejected, peer continued; both sandboxes stopped")
}

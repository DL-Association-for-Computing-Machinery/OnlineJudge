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
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCompileLeaseFaults(t *testing.T) {
	if os.Getenv("OJ_SANDBOX_COMPILE_LEASE") != "1" {
		t.Skip("run python3 scripts/test-worker.py compile-lease")
	}
	verifyEnvironment(t)
	if time.Duration(experiment.PinnedEnvironment().Compile.Wall) != 15*time.Second {
		t.Fatal("compile observation bound requires the validated 15 second wall limit")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := os.Getenv("OJ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("disposable database required")
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }() // Private fixture connection.
	var tables int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatal("compile-lease database must be empty")
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
	for _, fault := range []string{"network", "disabled"} {
		if !t.Run(fault, func(t *testing.T) { testCompileLeaseFault(t, pool, store, fault) }) {
			break
		}
	}
}

const compileLeaseSource = "#include <bits/stdc++.h>\nint main(){int a,b;std::cin>>a>>b;std::cout<<a+b;}\n"

func testCompileLeaseFault(t *testing.T, pool *pgxpool.Pool, store *postgres.Store, fault string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	id := dualDocker(t, "inspect", "--format", "{{.Id}}", "oj-m0-sandbox")
	peerID := dualDocker(t, "inspect", "--format", "{{.Id}}", "oj-m0-sandbox-secondary")
	restartDrainedSandbox(t, id)
	restartDrainedSandbox(t, peerID)
	node, peer := "compile-"+fault, "compile-peer-"+fault
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
			t.Error("compile fault relay did not drain")
		}
	})
	accept := func(key string) string {
		t.Helper()
		row, _, err := store.Accept(ctx, node, "m0-training", "compile-lease-"+fault+"-"+key,
			submission.Request{MaterialVersion: experiment.PinnedVersion, Language: "gnu++17", Source: compileLeaseSource})
		if err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	target, next := accept("target"), accept("next")
	// This is a lower bound on compilation start. Proving all stop evidence
	// before t0+14s excludes the unchanged 15s compiler wall and 20s curl timeout.
	t0 := time.Now()
	before := t0.Add(14 * time.Second)
	p := dualWorker(t, ca, certFile, key, relay.endpoint(), "primary")
	compiler := pauseFixtureCompiler(t, ctx, id, p, before)
	var token string
	if err := pool.QueryRow(ctx, `SELECT token FROM judge_attempts WHERE submission_id=$1 AND state='leased'`, target).Scan(&token); err != nil {
		t.Fatal(err)
	}
	// SIGSTOP itself must not immediately end the attempt. The compiler wall
	// keeps running during this short, controlled in-flight observation.
	time.Sleep(250 * time.Millisecond)
	p.alive(t)
	assertFixtureCompiler(t, id, compiler)
	if fault == "network" {
		relay.Block(relayUp | relayDown)
		for relay.upHeld.Load() == 0 {
			p.alive(t)
			if time.Now().After(before) {
				t.Fatal("no control traffic before compile observation cutoff")
			}
			time.Sleep(10 * time.Millisecond)
		}
		// Observe the same paused compiler after actual control bytes are held,
		// while the renewal request still has its normal three-second budget.
		p.alive(t)
		assertFixtureCompiler(t, id, compiler)
	} else {
		if _, err := pool.Exec(ctx, `UPDATE worker_nodes SET enabled=false WHERE id=$1`, node); err != nil {
			t.Fatal(err)
		}
		p.alive(t)
		assertFixtureCompiler(t, id, compiler)
	}
	timer := time.NewTimer(time.Until(before))
	defer timer.Stop()
	select {
	case err := <-p.done:
		p.stopped = true
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			t.Fatal("compile lease loss did not fail the worker")
		}
	case <-timer.C:
		t.Fatal("worker did not stop before the compile observation cutoff")
	}
	dualStopped(t, id, compiler.vmPID)
	elapsed := time.Since(t0)
	if !time.Now().Before(before) {
		t.Fatal("stop evidence arrived too late to exclude compiler wall timeout")
	}
	var attempts, results, nextAttempts int
	var nextState string
	if err := pool.QueryRow(ctx, `SELECT count(*),count(result_payload) FROM judge_attempts WHERE submission_id=$1`, target).Scan(&attempts, &results); err != nil || attempts != 1 || results != 0 {
		t.Fatal("interrupted compiler published or reran a result")
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM judge_tasks WHERE submission_id=$1`, next).Scan(&nextState); err != nil || nextState != "queued" {
		t.Fatal("failed worker claimed the next queued task")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM judge_attempts WHERE submission_id=$1`, next).Scan(&nextAttempts); err != nil || nextAttempts != 0 {
		t.Fatal("failed worker admitted a next attempt")
	}
	relay.Resume()
	_, peerCert, peerKey := ca.Issue(t, "spiffe://onlinejudge/worker/"+peer+"/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	healthy := dualWorker(t, ca, peerCert, peerKey, endpoints[1], "secondary")
	waitNetworkAC(t, ctx, store, node, target, healthy)
	waitNetworkAC(t, ctx, store, node, next, healthy)
	checkCompileRecovery(t, ctx, pool, ca, cert, endpoints, target, token, peer, fault)
	healthy.stop(t)
	dualStopped(t, peerID, "")
	t.Logf("%s: same compiler remained T through fault injection, all stop/PID evidence in %s (<14s from Worker start); no payload/next claim; peer recovered attempt2 AC with fixed generations and stale report rejected", fault, elapsed)
}

// Only this dedicated synthetic container is inspected/signalled. The PID here
// belongs to its parent namespace, never a VM PID returned by docker top.
const pauseCompilerScript = `set --
for compilerProc in /proc/[0-9]*/comm; do
    if read -r compilerName < "$compilerProc" && test "$compilerName" = cc1plus; then
        compilerPID=${compilerProc#/proc/}
        compilerPID=${compilerPID%/comm}
        case "$compilerPID" in ''|*[!0-9]*) exit 2;; esac
        set -- "$@" "$compilerPID"
    fi
done
if test "$#" -eq 0; then printf 'no-compiler\n' || exit 2; exit 0; fi
test "$#" -eq 1 || exit 2
compilerPID=$1
read -r compilerName < "/proc/$compilerPID/comm" || exit 2
test "$compilerName" = cc1plus || exit 2
kill -STOP "$compilerPID" || exit 2
printf '%s\n' "$compilerPID" || exit 2`

func pauseFixtureCompiler(t *testing.T, ctx context.Context, id string, p *dualProcess, before time.Time) compilerSnapshot {
	t.Helper()
	for time.Now().Before(before) {
		p.alive(t)
		probe, cancel := context.WithTimeout(ctx, time.Second)
		out, err := exec.CommandContext(probe, "docker", "--context", "desktop-linux", "exec", id, "/bin/sh", "-ec", pauseCompilerScript).Output()
		cancel()
		_, absent, evidenceErr := parseCompilerPause(string(out), err)
		if evidenceErr != nil {
			t.Fatal(evidenceErr)
		}
		if !absent {
			for time.Now().Before(before) {
				s, err := parseCompilerTop(dualDocker(t, "top", id, "-eo", "pid,comm,stat"))
				if err != nil || s.vmPID == "" {
					t.Fatal("paused compiler identity unavailable")
				}
				if strings.HasPrefix(s.state, "T") {
					return s
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("compiler could not be paused within the observation budget")
	return compilerSnapshot{}
}

func assertFixtureCompiler(t *testing.T, id string, want compilerSnapshot) {
	t.Helper()
	got, err := parseCompilerTop(dualDocker(t, "top", id, "-eo", "pid,comm,stat"))
	if err != nil || got.vmPID != want.vmPID || !strings.HasPrefix(got.state, "T") {
		t.Fatal("same paused compiler was not observed")
	}
}

func checkCompileRecovery(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ca *testpki.Authority, cert tls.Certificate, endpoints []string, target, token, peer, fault string) {
	t.Helper()
	var payload []byte
	var owner, state string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT node_id,result_payload FROM judge_attempts WHERE submission_id=$1 AND state='completed' AND attempt_number=2 AND judge_generation=1 AND recovery_generation=1`, target).Scan(&owner, &payload); err != nil || owner != peer {
		t.Fatal("peer did not recover attempt2 in the original generations")
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM judge_attempts WHERE token=$1`, token).Scan(&state); err != nil || state != "expired" {
		t.Fatal("interrupted attempt did not expire")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM judge_attempts WHERE submission_id=$1`, target).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatal("unexpected compile recovery attempts")
	}
	var result submission.Result
	if err := json.Unmarshal(payload, &result); err != nil || result.Verdict != "AC" {
		t.Fatal("peer did not preserve a real AC payload")
	}
	want := submission.ErrLease
	if fault == "disabled" {
		want = submission.ErrNode
	}
	// This is a late authorization probe with the peer's actual result, not a
	// claim that the compiler-interrupted Worker ever produced a frozen result.
	for _, endpoint := range endpoints {
		r, err := workerclient.NewReporter(endpoint, ca.Roots, cert)
		if err != nil {
			t.Fatal(err)
		}
		err = r.Report(ctx, token, result)
		r.Close()
		if !errors.Is(err, want) {
			t.Fatal("interrupted compiler identity/result was accepted")
		}
	}
	var same []byte
	if err := pool.QueryRow(ctx, `SELECT result_payload FROM judge_attempts WHERE submission_id=$1 AND state='completed'`, target).Scan(&same); err != nil || string(same) != string(payload) {
		t.Fatal("late probe changed recovered evidence")
	}
}

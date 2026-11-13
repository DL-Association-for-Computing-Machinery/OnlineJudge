//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
)

const (
	aplusbNode    = "aplusb-fixture-node"
	aplusbSubject = "synthetic-aplusb"
	aplusbScope   = submission.PersonalTrainingScope
	aplusbAC      = "#include <iostream>\nint main(){long long a,b;std::cin>>a>>b;std::cout<<a+b<<'\\n';}\n"
)

func aplusbEnvironment() []string {
	return []string{"OJ_SANDBOX_EXPERIMENT=1", "OJ_APLUSB_MANIFEST_FILE=" + os.Getenv("OJ_APLUSB_MANIFEST_FILE"),
		"OJ_APLUSB_MANIFEST_SHA256=" + os.Getenv("OJ_APLUSB_MANIFEST_SHA256"), "OJ_APLUSB_RUNTIME=" + os.Getenv("OJ_APLUSB_RUNTIME")}
}

func aplusbDBEnvironment(dsn, root string) []string {
	return append(aplusbEnvironment(), "OJ_DATABASE_URL="+dsn, "OJ_MATERIAL_ROOT="+root)
}

func migrateAPlusB(t *testing.T, ctx context.Context, dsn, root string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, "../../.local/bin/migrate", args...)
	cmd.Env = httpLoopEnvironment(aplusbDBEnvironment(dsn, root)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("A+B fixture setup failed: %v: %s", err, out)
	}
}

func startAPlusBController(t *testing.T, dsn, root string, ca *testpki.Authority) (string, *dualProcess) {
	t.Helper()
	_, cert, key := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := reserved.Addr().String()
	_ = reserved.Close()
	return "https://" + address, startDualProcess(t, "worker-api", httpLoopEnvironment(append(aplusbDBEnvironment(dsn, root),
		"OJ_WORKER_API_ADDR="+address, "OJ_WORKER_CA_FILE="+ca.CAFile, "OJ_WORKER_CERT_FILE="+cert, "OJ_WORKER_KEY_FILE="+key)...))
}

func aplusbWorkerIdentity(t *testing.T, ca *testpki.Authority) (tls.Certificate, string, string) {
	t.Helper()
	return ca.Issue(t, "spiffe://onlinejudge/worker/"+aplusbNode+"/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
}

func startAPlusBWorker(t *testing.T, endpoint, ca, cert, key string) *dualProcess {
	t.Helper()
	return startDualProcess(t, "worker", httpLoopEnvironment(append(aplusbEnvironment(), "OJ_APLUSB_START_STOPPED=1", "OJ_WORKER_CONTROLLER_URL="+endpoint,
		"OJ_WORKER_CA_FILE="+ca, "OJ_WORKER_CERT_FILE="+cert, "OJ_WORKER_KEY_FILE="+key)...))
}

func waitAPlusBVerdict(t *testing.T, ctx context.Context, worker *dualProcess, store *postgres.Store, initial submission.Receipt, want string) {
	t.Helper()
	waitAPlusBReceipt(t, ctx, worker, initial, want, func() (submission.Receipt, error) {
		return store.Get(ctx, aplusbSubject, aplusbScope, initial.ID)
	})
}

func waitAPlusBReceipt(t *testing.T, ctx context.Context, worker *dualProcess, initial submission.Receipt, want string, read func() (submission.Receipt, error)) {
	t.Helper()
	for ctx.Err() == nil {
		worker.alive(t)
		row, err := read()
		if err != nil || row.ID != initial.ID || row.MaterialVersion != initial.MaterialVersion ||
			row.Language != initial.Language || !row.CreatedAt.Equal(initial.CreatedAt) {
			t.Fatal("A+B submission identity changed")
		}
		if row.State == "finished" {
			if row.Verdict == nil || *row.Verdict != want {
				t.Fatal("unexpected A+B judgment")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("Worker did not finish A+B within budget")
}

func drainAPlusB(t *testing.T, ctx context.Context, worker *dualProcess) {
	t.Helper()
	if err := worker.cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		t.Fatal("cannot request A+B Worker drain")
	}
	waitDrainedProcess(t, worker, ctx)
}

// The driver supplies a disposable database and the isolated, stopped
// reference slot. This test never opens a browser or runs a public API.
func TestAPlusBLeasedLoop(t *testing.T) {
	if os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv("OJ_SANDBOX_APLUSB") != "1" {
		t.Skip("requires explicit isolated A+B test setup")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	m, raw, enabled, err := materials.ConfiguredAPlusB()
	if err != nil || !enabled || os.Getenv("OJ_TEST_DATABASE_URL") == "" || os.Getenv("OJ_MATERIAL_ROOT") == "" {
		t.Fatal("private material and disposable database configuration required")
	}
	dsn := os.Getenv("OJ_TEST_DATABASE_URL")
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var tables int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'").Scan(&tables); err != nil || tables != 0 {
		t.Fatal("A+B integration database must be empty")
	}
	root := os.Getenv("OJ_MATERIAL_ROOT")
	for _, args := range [][]string{{"up"}, {"backlog", "3", "3"}} {
		migrateAPlusB(t, ctx, dsn, root, args...)
	}
	missing := exec.CommandContext(ctx, "../../.local/bin/migrate", "fixture")
	missing.Env = httpLoopEnvironment(aplusbDBEnvironment(dsn, t.TempDir())...)
	if err := missing.Run(); err == nil {
		t.Fatal("registration accepted missing material bytes")
	}
	var versions int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM submission_materials").Scan(&versions); err != nil || versions != 0 {
		t.Fatal("failed registration left a material version")
	}
	ubuntu := os.Getenv("OJ_APLUSB_RUNTIME") == "ubuntu-arm64"
	if ubuntu {
		// A correctly hashed private manifest must still be refused by default
		// Desktop registration, before any material version is written.
		wrong := exec.CommandContext(ctx, "../../.local/bin/migrate", "fixture")
		wrong.Env = httpLoopEnvironment(append(aplusbDBEnvironment(dsn, root), "OJ_APLUSB_RUNTIME=")...)
		if err := wrong.Run(); err == nil {
			t.Fatal("Desktop registration admitted the Ubuntu material")
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM submission_materials").Scan(&versions); err != nil || versions != 0 {
			t.Fatal("wrong runtime left a material version")
		}
	}
	for range 2 {
		migrateAPlusB(t, ctx, dsn, root, "fixture")
	}
	store, err := postgres.New(pool, m.Version, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)", aplusbNode); err != nil {
		t.Fatal(err)
	}
	sources := []string{
		aplusbAC,
		"#include <iostream>\nint main(){std::cout<<\"0\\n\";}\n",
		"#include <iostream>\nint main(){std::cout<<\"1111111111111\\n\";}\n",
	}
	if m.TokenBytes != 12 {
		t.Fatal("regression expects the reviewed 12-byte token configuration")
	}
	receipts := make([]submission.Receipt, 0, len(sources))
	var trainingActor identity.TrainingActor
	for i, source := range sources {
		key := "aplusb-fixture-key-" + string(rune('a'+i))
		if i == 0 {
			actor, row := acceptAPlusBTraining(t, ctx, pool, store, key, source)
			trainingActor = actor
			receipts = append(receipts, row)
			continue
		}
		row, created, err := store.Accept(ctx, aplusbSubject, aplusbScope, key,
			submission.Request{MaterialVersion: m.Version, Language: "gnu++17", Source: source})
		if err != nil || !created {
			t.Fatalf("accept synthetic task: %v", err)
		}
		receipts = append(receipts, row)
	}
	ca := testpki.New(t)
	pair, clientCert, clientKey := aplusbWorkerIdentity(t, ca)
	endpoint, controller := startAPlusBController(t, dsn, root, ca)
	old, err := workerclient.NewAcquirer(endpoint, ca.Roots, pair)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	for {
		controller.alive(t)
		if _, err := old.Claim(ctx); errors.Is(err, workerclient.ErrMaterials) {
			break // The old fixture assertion was rejected before claiming.
		}
		if ctx.Err() != nil {
			t.Fatal("wrong fixture never explicitly refused")
		}
		time.Sleep(50 * time.Millisecond)
	}
	var attempts int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM judge_attempts").Scan(&attempts); err != nil || attempts != 0 {
		t.Fatal("wrong fixture created an attempt")
	}
	if ubuntu {
		assertDesktopAPlusBClaimRejected(t, ctx, endpoint, ca.Roots, pair)
		assertUbuntuAPlusBManifestMismatch(t, ctx, endpoint, ca.Roots, pair, m)
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM judge_attempts").Scan(&attempts); err != nil || attempts != 0 {
			t.Fatal("Desktop A+B runtime created an Ubuntu attempt")
		}
	}
	worker := startAPlusBWorker(t, endpoint, ca.CAFile, clientCert, clientKey)
	for i, want := range []string{"AC", "WA", "OLE"} {
		if i == 0 {
			row := receipts[i]
			waitAPlusBReceipt(t, ctx, worker, row, want, func() (submission.Receipt, error) {
				return postgres.ReadTrainingStatus(ctx, pool, trainingActor, row.ID)
			})
			detail, err := postgres.ReadTrainingDetail(ctx, pool, trainingActor, row.ID)
			if err != nil || detail.Source != sources[i] || detail.Verdict == nil || *detail.Verdict != want {
				t.Fatal("training owner did not recover the source and real Worker verdict")
			}
			continue
		}
		waitAPlusBVerdict(t, ctx, worker, store, receipts[i], want)
	}
	sum := sha256.Sum256(raw)
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM judge_attempts a JOIN submissions s ON s.id=a.submission_id
		WHERE a.state='completed' AND a.attempt_number=1 AND s.material_version=$1 AND s.manifest_sha256=$2`,
		m.Version, hex.EncodeToString(sum[:])).Scan(&attempts); err != nil || attempts != 3 {
		t.Fatal("completed attempts did not retain the exact material binding")
	}
	drainAPlusB(t, ctx, worker)
	controller.stop(t)
	t.Log("authenticated store accepted fixed A+B, owner read real AC, wrong fixture refused, WA/OLE and drain confirmed")
}

func assertUbuntuAPlusBManifestMismatch(t *testing.T, ctx context.Context, endpoint string, roots *x509.CertPool, pair tls.Certificate, m materials.APlusBManifest) {
	t.Helper()
	// Version alone cannot distinguish two differently pinned transport projections.
	m.TokenBytes++
	raw := m.Canonical()
	sum := sha256.Sum256(raw)
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"OJ_SANDBOX_EXPERIMENT": "1", "OJ_APLUSB_RUNTIME": "ubuntu-arm64",
		"OJ_APLUSB_MANIFEST_FILE": path, "OJ_APLUSB_MANIFEST_SHA256": hex.EncodeToString(sum[:]),
	}
	_, fixture, err := experiment.LoadAPlusBFixture(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := workerclient.NewAcquirerForFixture(endpoint, roots, pair, fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.Close()
	if _, err := wrong.Claim(ctx); !errors.Is(err, workerclient.ErrMaterials) {
		t.Fatal("the Ubuntu controller accepted a different manifest with the same version")
	}
}

func assertDesktopAPlusBClaimRejected(t *testing.T, ctx context.Context, endpoint string, roots *x509.CertPool, pair tls.Certificate) {
	t.Helper()
	raw, err := os.ReadFile(os.Getenv("OJ_TEST_APLUSB_DESKTOP_MANIFEST"))
	if err != nil {
		t.Fatal("Ubuntu comparison requires the original Desktop manifest")
	}
	sum := sha256.Sum256(raw)
	f, err := experiment.APlusBFixture(raw, hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	old, err := workerclient.NewAcquirerForFixture(endpoint, roots, pair, f)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if _, err := old.Claim(ctx); !errors.Is(err, workerclient.ErrMaterials) {
		t.Fatal("Ubuntu controller did not refuse the Desktop A+B fixture")
	}
}

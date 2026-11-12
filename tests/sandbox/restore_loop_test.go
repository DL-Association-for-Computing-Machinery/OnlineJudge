//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	restoreNode    = "restore-loop-node"
	restoreSubject = "restore-loop-subject"
	restoreScope   = "m0-training"
	restoreKey     = "restore-loop-key-0001"
)

func TestRestoreJudgeLoop(t *testing.T) {
	if os.Getenv("OJ_SANDBOX_RESTORE") != "1" {
		t.Skip("run python3 scripts/test-worker.py restore-loop")
	}
	verifyEnvironment(t)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	source, target := restoreLoopDatabases(t, ctx)
	const version = experiment.PinnedVersion
	_, manifest, err := experiment.SelectFixture(version)
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot, backupRoot, targetRoot := t.TempDir(), t.TempDir(), t.TempDir()
	backupMaterials := filepath.Join(backupRoot, "materials")
	if err := os.Mkdir(backupMaterials, 0700); err != nil {
		t.Fatal(err)
	}
	migrateHTTPFixture(t, ctx, source.dsn, sourceRoot, version, "2")
	if _, err := source.pool.Exec(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)`, restoreNode); err != nil {
		t.Fatal("cannot register the synthetic Worker")
	}
	original, err := postgres.New(source.pool, version, manifest)
	if err != nil {
		t.Fatal(err)
	}
	request := submission.Request{MaterialVersion: version, Language: "gnu++17",
		Source: "// 合成恢复夹具\r\n#include <cstdio>\nint main(){int a,b;if(std::scanf(\"%d%d\",&a,&b)!=2)return 1;std::printf(\"%d\\n\",a+b);}\n"}
	accepted, created, err := original.Accept(ctx, restoreSubject, restoreScope, restoreKey, request)
	if err != nil || !created || accepted.State != "queued" {
		t.Fatal("cannot accept the original synthetic submission")
	}
	ca := testpki.New(t)
	cert, certFile, keyFile := ca.Issue(t, "spiffe://onlinejudge/worker/"+restoreNode+"/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	judge := func(database restoreLoopDatabase, root string, store *postgres.Store) {
		t.Helper()
		controller, process := controllerForDatabase(t, ca, cert, root, version, database.dsn)
		stop := httpLoopLifetime(t, version)
		worker := dualWorker(t, ca, certFile, keyFile, controller, "primary")
		for ctx.Err() == nil {
			worker.alive(t)
			receipt, err := store.Get(ctx, restoreSubject, restoreScope, accepted.ID)
			if err != nil || receipt.ID != accepted.ID || receipt.MaterialVersion != version || !receipt.CreatedAt.Equal(accepted.CreatedAt) {
				t.Fatal("submission identity changed while waiting for Worker")
			}
			if receipt.State == "finished" {
				if receipt.Verdict == nil || *receipt.Verdict != "AC" {
					t.Fatal("expected real AC for the restored fixture")
				}
				stop(t, worker)
				process.stop(t)
				return
			}
			if (receipt.State != "queued" && receipt.State != "running") || receipt.Verdict != nil {
				t.Fatal("unexpected Worker state or verdict")
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("real Worker did not produce a terminal result")
	}
	slotID := dualDocker(t, "inspect", "--format", "{{.Id}}", container)
	judge(source, sourceRoot, original)
	history := restoreAttempt(t, ctx, source.pool, accepted.ID, 1)

	// Stop the source cluster after capturing the quiescent fixture. No live source
	// database can serve the second judgment, and no fixture is seeded on the target.
	const archivePath = "/tmp/oj-restore-loop.dump"
	archive := filepath.Join(backupRoot, "submission.dump")
	dualDocker(t, "exec", source.container, "pg_dump", "-U", "oj_test", "-Fc", "-f", archivePath, source.name)
	dualDocker(t, "cp", source.container+":"+archivePath, archive)
	copyRestoreMaterials(t, sourceRoot, backupMaterials)
	dualDocker(t, "stop", source.container)
	if dualDocker(t, "inspect", "--format", "{{.State.Running}}", source.container) != "false" {
		t.Fatal("source database stop not confirmed")
	}
	dualDocker(t, "cp", archive, target.container+":"+archivePath)
	dualDocker(t, "exec", target.container, "pg_restore", "-U", "oj_test", "--single-transaction", "--no-owner", "--no-privileges", "-d", target.name, archivePath)
	copyRestoreMaterials(t, backupMaterials, targetRoot)
	if err := db.CheckSchema(ctx, target.pool); err != nil {
		t.Fatal(err)
	}
	if restoreAttempt(t, ctx, target.pool, accepted.ID, 1) != history {
		t.Fatal("historical attempt changed in the logical restore")
	}
	var sourceText string
	if err := target.pool.QueryRow(ctx, `SELECT source_code FROM submissions WHERE id=$1`, accepted.ID).Scan(&sourceText); err != nil || sourceText != request.Source {
		t.Fatal("restored source did not match byte for byte")
	}
	restored, err := postgres.New(target.pool, version, manifest)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := restored.Receipt(ctx, restoreSubject, restoreScope, restoreKey)
	if err != nil || receipt.ID != accepted.ID || receipt.State != "finished" || receipt.Verdict == nil || *receipt.Verdict != "AC" {
		t.Fatal("restored original-key history changed")
	}
	// Test-only scheduling: the product does not expose a rejudge command yet.
	// The isolated fixture has one task and retains its original queue order.
	changed, err := target.pool.Exec(ctx, `UPDATE judge_tasks SET state='queued', verdict=NULL, attempt_number=0, judge_generation=2
		WHERE submission_id=$1 AND state='finished' AND verdict='AC' AND attempt_number=1 AND judge_generation=1 AND recovery_generation=1`, accepted.ID)
	if err != nil || changed.RowsAffected() != 1 {
		t.Fatal("cannot advance the restored fixture to a new judge generation")
	}
	restartDrainedSandbox(t, slotID)
	verifyEnvironment(t)
	judge(target, targetRoot, restored)
	if restoreAttempt(t, ctx, target.pool, accepted.ID, 1) != history {
		t.Fatal("real rejudge changed the historical attempt")
	}
	restoreAttempt(t, ctx, target.pool, accepted.ID, 2)
	var submissions, attempts int
	if err := target.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM submissions), (SELECT count(*) FROM judge_attempts)`).Scan(&submissions, &attempts); err != nil || submissions != 1 || attempts != 2 {
		t.Fatal("rejudge must not resubmit or create extra attempts")
	}
	t.Logf("one submission, two real AC generations, historical result unchanged; source SHA-256=%x; source database stopped", sha256.Sum256([]byte(sourceText)))
}

type restoreLoopDatabase struct {
	container, name, dsn string
	pool                 *pgxpool.Pool
}

func restoreLoopDatabases(t *testing.T, ctx context.Context) (restoreLoopDatabase, restoreLoopDatabase) {
	t.Helper()
	name, from, to := os.Getenv("OJ_RESTORE_DATABASE"), os.Getenv("OJ_RESTORE_SOURCE_CONTAINER"), os.Getenv("OJ_RESTORE_TARGET_CONTAINER")
	containerName := regexp.MustCompile(`^oj-worker-db-[a-f0-9]{12}$`)
	if from == to || !containerName.MatchString(from) || !containerName.MatchString(to) || !regexp.MustCompile(`^test_[0-9]+$`).MatchString(name) {
		t.Fatal("two owned, disposable database containers are required")
	}
	endpoint := regexp.MustCompile(`^127\.0\.0\.1:([0-9]+)$`)
	open := func(container string) restoreLoopDatabase {
		t.Helper()
		if dualDocker(t, "inspect", "--format", `{{index .Config.Labels "owner"}}`, container) != "onlinejudge.desktop-test" {
			t.Fatal("refusing to use an unowned database container")
		}
		address := endpoint.FindStringSubmatch(dualDocker(t, "port", container, "5432"))
		if len(address) != 2 {
			t.Fatal("disposable database is not loopback only")
		}
		dsn := "host=127.0.0.1 port=" + address[1] + " user=oj_test dbname=" + name + " sslmode=disable"
		pool, err := postgres.Open(ctx, dsn)
		if err != nil {
			t.Fatal("cannot connect to a private database")
		}
		t.Cleanup(pool.Close)
		var tables int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&tables); err != nil || tables != 0 {
			t.Fatal("restore loop databases must start empty")
		}
		return restoreLoopDatabase{container: container, name: name, dsn: dsn, pool: pool}
	}
	return open(from), open(to)
}

type restoredAttempt struct{ fingerprint, payload string }

func restoreAttempt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string, generation int) restoredAttempt {
	t.Helper()
	var attempt restoredAttempt
	if err := pool.QueryRow(ctx, `SELECT result_sha256, result_payload::text FROM judge_attempts
		WHERE submission_id=$1 AND judge_generation=$2 AND recovery_generation=1 AND attempt_number=1 AND node_id=$3
		AND state='completed' AND result_payload->>'Verdict'='AC'`, id, generation, restoreNode).Scan(&attempt.fingerprint, &attempt.payload); err != nil {
		t.Fatal("cannot read the expected completed Worker attempt")
	}
	return attempt
}

func copyRestoreMaterials(t *testing.T, from, to string) {
	t.Helper()
	entries, err := os.ReadDir(from)
	if err != nil {
		t.Fatal(err)
	}
	digest := regexp.MustCompile(`^[a-f0-9]{64}$`)
	count := 0
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !digest.MatchString(entry.Name()) {
			t.Fatal("unexpected file in synthetic material directory")
		}
		data, err := os.ReadFile(filepath.Join(from, entry.Name()))
		sum := sha256.Sum256(data)
		if err != nil || len(data) == 0 || len(data) > 128 || hex.EncodeToString(sum[:]) != entry.Name() {
			t.Fatal("cannot read bounded synthetic material")
		}
		if err := os.WriteFile(filepath.Join(to, entry.Name()), data, 0600); err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count != 4 {
		t.Fatal("expected four backed-up synthetic material files")
	}
}

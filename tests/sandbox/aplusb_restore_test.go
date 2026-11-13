//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/sha256"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAPlusBRestoreJudgeLoop(t *testing.T) {
	if os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv("OJ_SANDBOX_APLUSB_RESTORE") != "1" {
		t.Skip("run the isolated aplusb-restore-loop suite")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	manifest, raw, enabled, err := materials.ConfiguredAPlusB()
	root, pin := os.Getenv("OJ_MATERIAL_ROOT"), os.Getenv("OJ_APLUSB_MANIFEST_SHA256")
	if err != nil || !enabled || root == "" {
		t.Fatal("explicit private A+B objects and trusted manifest pin required")
	}
	source, target := restoreLoopDatabases(t, ctx)
	for _, args := range [][]string{{"up"}, {"backlog", "2", "2"}, {"fixture"}} {
		migrateAPlusB(t, ctx, source.dsn, root, args...)
	}
	if _, err := source.pool.Exec(ctx, "INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)", aplusbNode); err != nil {
		t.Fatal("cannot register the synthetic A+B Worker")
	}
	store, err := postgres.New(source.pool, manifest.Version, raw)
	if err != nil {
		t.Fatal(err)
	}
	const key = "aplusb-restore-key-0001"
	request := submission.Request{MaterialVersion: manifest.Version, Language: "gnu++17", Source: aplusbAC}
	initial, created, err := store.Accept(ctx, aplusbSubject, aplusbScope, key, request)
	if err != nil || !created || initial.State != "queued" || initial.Verdict != nil {
		t.Fatal("cannot accept the original A+B fixture")
	}
	slot := (workerhost.ReferenceSlot{}).ContainerName()
	slotID := dualDocker(t, "inspect", "--format", "{{.Id}}", slot)
	judge := func(database restoreLoopDatabase, objects string, binding *postgres.Store, checkContention bool) {
		t.Helper()
		ca := testpki.New(t)
		_, cert, key := aplusbWorkerIdentity(t, ca)
		endpoint, control := startAPlusBController(t, database.dsn, objects, ca)
		worker := startAPlusBWorker(t, endpoint, ca.CAFile, cert, key)
		waitAPlusBVerdict(t, ctx, worker, binding, initial, "AC")
		if checkContention {
			assertAPlusBReferenceBusy(t, ctx, slotID, endpoint, ca.CAFile, cert, key)
		}
		drainAPlusB(t, ctx, worker)
		control.stop(t)
		if dualDocker(t, "inspect", "--format", "{{.Id}}", slot) != slotID {
			t.Fatal("fixed A+B sandbox was replaced")
		}
		dualStopped(t, slotID, "")
	}
	judge(source, root, store, true)
	first := aplusbRestoredAttempt(t, ctx, source.pool, initial.ID, 1)

	objects := restoreSealedPackage(t, source, target, root,
		restoreMaterialExpectation{files: 32, references: 34, checkMissing: true})
	restored, err := postgres.New(target.pool, manifest.Version, raw)
	if err != nil || restored.CheckRegisteredMaterial(ctx) != nil {
		t.Fatal("restored version does not match the original trusted pin")
	}
	receipt, err := restored.Receipt(ctx, aplusbSubject, aplusbScope, key)
	if err != nil || receipt.ID != initial.ID || receipt.State != "finished" || receipt.Verdict == nil || *receipt.Verdict != "AC" ||
		receipt.MaterialVersion != initial.MaterialVersion || !receipt.CreatedAt.Equal(initial.CreatedAt) {
		t.Fatal("restored original-key receipt changed")
	}
	var text, version, hash string
	if err := target.pool.QueryRow(ctx, "SELECT source_code, material_version, manifest_sha256 FROM submissions WHERE id=$1", initial.ID).
		Scan(&text, &version, &hash); err != nil || text != request.Source || version != manifest.Version || hash != pin {
		t.Fatal("restored source bytes or material binding changed")
	}
	if aplusbRestoredAttempt(t, ctx, target.pool, initial.ID, 1) != first {
		t.Fatal("historical A+B attempt changed during restore")
	}

	// Test-only scheduling in the owned target: there is no product rejudge API.
	changed, err := target.pool.Exec(ctx, `UPDATE judge_tasks SET state='queued', verdict=NULL, attempt_number=0, judge_generation=2
		WHERE submission_id=$1 AND state='finished' AND verdict='AC' AND attempt_number=1 AND judge_generation=1 AND recovery_generation=1`, initial.ID)
	if err != nil || changed.RowsAffected() != 1 {
		t.Fatal("cannot advance the restored A+B fixture to a new judge generation")
	}
	judge(target, objects, restored, false)
	if aplusbRestoredAttempt(t, ctx, target.pool, initial.ID, 1) != first {
		t.Fatal("rejudge changed the historical A+B result")
	}
	second := aplusbRestoredAttempt(t, ctx, target.pool, initial.ID, 2)
	if first.fingerprint == second.fingerprint || first.payload == second.payload {
		t.Fatal("second generation did not produce its own result evidence")
	}
	var submissions, attempts int
	if err := target.pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM submissions), (SELECT count(*) FROM judge_attempts)").
		Scan(&submissions, &attempts); err != nil || submissions != 1 || attempts != 2 {
		t.Fatal("rejudge resubmitted or created extra attempts")
	}
	t.Logf("one A+B submission, two AC generations, original result preserved; material=%s; source=%x", pin, sha256.Sum256([]byte(text)))
}

func assertAPlusBReferenceBusy(t *testing.T, ctx context.Context, slotID, endpoint, ca, cert, key string) {
	t.Helper()
	const state = "{{.State.Running}} {{.State.Pid}} {{.State.StartedAt}}"
	owner := dualDocker(t, "inspect", "--format", state, slotID)
	contender := startAPlusBWorker(t, endpoint, ca, cert, key)
	select {
	case err := <-contender.done:
		contender.stopped = true
		log, readErr := os.ReadFile(contender.logPath)
		if err == nil || readErr != nil || !strings.Contains(string(log), "host execution slot unavailable") {
			t.Fatal("second Worker did not explicitly refuse the occupied canonical slot")
		}
	case <-ctx.Done():
		t.Fatal("second Worker did not refuse the occupied slot")
	}
	if !strings.HasPrefix(owner, "true ") || dualDocker(t, "inspect", "--format", state, slotID) != owner {
		t.Fatal("second Worker changed the first Worker's sandbox lifetime")
	}
	t.Log("second Worker refused occupied canonical slot; existing sandbox lifetime unchanged")
}

func aplusbRestoredAttempt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string, generation int) restoredAttempt {
	t.Helper()
	var result restoredAttempt
	var evidence string
	if err := pool.QueryRow(ctx, `SELECT result_sha256, result_payload::text, result_payload->>'EvidenceSHA256' FROM judge_attempts
		WHERE submission_id=$1 AND judge_generation=$2 AND recovery_generation=1 AND attempt_number=1 AND node_id=$3
		AND state='completed' AND result_payload->>'Verdict'='AC'`, id, generation, aplusbNode).
		Scan(&result.fingerprint, &result.payload, &evidence); err != nil || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(evidence) {
		t.Fatal("cannot read the expected completed A+B generation")
	}
	return result
}

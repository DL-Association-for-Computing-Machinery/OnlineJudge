//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"os/exec"
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

	rejudgeRestoredAPlusB(t, ctx, target, objects, initial, pin)
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

func rejudgeRestoredAPlusB(t *testing.T, ctx context.Context, target restoreLoopDatabase, objects string, initial submission.Receipt, pin string) {
	t.Helper()
	for _, ddl := range []string{
		"CREATE ROLE oj_rejudge_operator NOLOGIN",
		"CREATE ROLE oj_restore_rejudge LOGIN",
		"CREATE ROLE oj_indirect_rejudge NOLOGIN",
		"CREATE ROLE oj_restore_readonly LOGIN",
		"GRANT oj_rejudge_operator TO oj_restore_rejudge",
		"GRANT oj_rejudge_operator TO oj_indirect_rejudge",
		"GRANT oj_indirect_rejudge TO oj_restore_readonly",
		"GRANT SELECT ON goose_db_version, submission_materials, judge_tasks, submission_backlog_policy, training_queue_clock, submission_rejudges TO oj_rejudge_operator",
		"GRANT SELECT (id, subject_id, context_id, material_version, manifest_sha256) ON submissions TO oj_rejudge_operator",
		"GRANT SELECT (submission_id, judge_generation, recovery_generation, attempt_number, state, result_payload) ON judge_attempts TO oj_rejudge_operator",
		"GRANT UPDATE (singleton) ON submission_backlog_policy TO oj_rejudge_operator",
		"GRANT UPDATE (floor) ON training_queue_clock TO oj_rejudge_operator",
		"GRANT UPDATE (state, verdict, attempt_number, judge_generation, queue_version, virtual_finish, enqueue_order) ON judge_tasks TO oj_rejudge_operator",
		"GRANT INSERT ON submission_rejudges TO oj_rejudge_operator",
		"GRANT USAGE ON SEQUENCE training_enqueue_order TO oj_rejudge_operator",
	} {
		if _, err := target.pool.Exec(ctx, ddl); err != nil {
			t.Fatal("cannot grant the disposable maintenance role")
		}
	}
	dsn := strings.Replace(target.dsn, "user=oj_test ", "user=oj_restore_rejudge ", 1)
	indirect := strings.Replace(target.dsn, "user=oj_test ", "user=oj_restore_readonly ", 1)
	const key = "restored-aplusb-rejudge-01"
	run := func(dsn, root string, args ...string) ([]byte, error) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "../../.local/bin/migrate", append([]string{"rejudge"}, args...)...)
		cmd.Env = httpLoopEnvironment("OJ_REJUDGE_DATABASE_URL="+dsn, "OJ_MATERIAL_ROOT="+root)
		return cmd.CombinedOutput()
	}
	args := []string{"enqueue", target.name, initial.ID, "1", initial.MaterialVersion, pin, key, "verify restored result", "--confirm-rejudge"}
	if out, err := run(indirect, objects, args...); err == nil || !strings.Contains(string(out), "rejudge requires an authorized database login") {
		t.Fatal("maintenance command did not reject indirect role membership")
	}
	if out, err := run(dsn, t.TempDir(), args...); err == nil || !strings.Contains(string(out), "material unavailable or inconsistent") {
		t.Fatal("maintenance command did not reject missing original materials")
	}
	out, err := run(dsn, objects, args...)
	if err != nil {
		t.Fatalf("rejudge command failed: %s", out)
	}
	var receipt postgres.RejudgeReceipt
	if json.Unmarshal(out, &receipt) != nil || receipt.SubmissionID != initial.ID || receipt.Generation != 2 ||
		receipt.Operator != "oj_restore_rejudge" || receipt.Key != key || receipt.Manifest != pin {
		t.Fatal("maintenance audit does not match restored submission")
	}
	again, err := run(dsn, "", "status", target.name, key)
	if err != nil || string(again) != string(out) {
		t.Fatal("could not recover the original maintenance receipt")
	}
	t.Log("authorized maintenance command queued the restored submission; original key returned its immutable audit")
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

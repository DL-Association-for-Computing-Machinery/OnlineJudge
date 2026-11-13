//go:build sandbox

package sandbox_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/backup"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/restorecheck"
)

type restoreMaterialExpectation struct {
	files, references int64
	checkMissing      bool
}

const (
	restoreStage = "/tmp/oj-worker-restore"
	restoreTool  = restoreStage + "/migrate"
	restoreSock  = "/var/run/postgresql"
)

// Only called after restoreLoopDatabases checks both owned, empty containers.
// The target copy must pass the independent manifest check before pg_restore.
func restoreSealedPackage(t *testing.T, source, target restoreLoopDatabase, sourceRoot string, want restoreMaterialExpectation) string {
	t.Helper()
	linuxTool, err := filepath.Abs("../../.local/bin/migrate-linux")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(linuxTool)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0100 == 0 {
		t.Fatal("the separate Linux restore artifact is required")
	}
	for _, container := range []string{source.container, target.container} {
		dualDocker(t, "exec", "--user", "0", container, "mkdir", "-m", "700", restoreStage)
		dualDocker(t, "cp", linuxTool, container+":"+restoreTool)
	}
	judge := restoreStage + "/judge"
	private := restoreStage + "/private"
	sealed := restoreStage + "/sealed"
	dualDocker(t, "exec", "--user", "0", source.container, "mkdir", "-m", "700", judge, private)
	dualDocker(t, "cp", sourceRoot+"/.", source.container+":"+judge)
	start := time.Now()
	var captured backup.Manifest
	readRestoreReceipt(t, dualDocker(t, "exec", "--user", "0", source.container, restoreTool, "capture-backup",
		"-socket", restoreSock, "-port", "5432", "-user", "oj_test", "-timeout", "8s", source.name, judge, private, sealed), &captured)
	captureElapsed := time.Since(start)
	if captured.Database != source.name || captured.JudgeFiles != want.files || captured.PrivateFiles != 0 ||
		captured.References.JudgeManifests != 1 || captured.References.JudgeReferences != want.references ||
		captured.References.Drafts != 0 || captured.Archive.Bytes <= 0 {
		t.Fatal("captured package did not match the synthetic source")
	}
	// Record the digest on the source side before any transfer; the recipient
	// must not derive the expected value from its own copy of the manifest.
	line := dualDocker(t, "exec", "--user", "0", source.container, "sha256sum", sealed+"/manifest.json")
	trusted, _, _ := strings.Cut(line, " ")
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(trusted) {
		t.Fatal("source did not return a valid completion digest")
	}
	start = time.Now()
	received := filepath.Join(t.TempDir(), "received")
	dualDocker(t, "cp", source.container+":"+sealed, received)
	dualDocker(t, "stop", source.container)
	if dualDocker(t, "inspect", "--format", "{{.State.Running}}", source.container) != "false" {
		t.Fatal("source database stop not confirmed")
	}
	targetCopy := restoreStage + "/received"
	dualDocker(t, "cp", received, target.container+":"+targetCopy)
	var verified backup.TransferReceipt
	readRestoreReceipt(t, dualDocker(t, "exec", "--user", "0", target.container, restoreTool, "verify-backup",
		"-manifest-sha256", trusted, "-timeout", "8s", targetCopy), &verified)
	if verified.ManifestSHA256 != trusted || verified.Archive != captured.Archive ||
		verified.JudgeFiles != captured.JudgeFiles || verified.PrivateFiles != captured.PrivateFiles {
		t.Fatal("received backup was not the source-sealed package")
	}
	transferElapsed := time.Since(start)
	start = time.Now()
	dualDocker(t, "exec", "--user", "0", target.container, "pg_restore", "--host", restoreSock, "--port", "5432", "--username", "oj_test", "--no-password",
		"--single-transaction", "--no-owner", "--no-privileges", "--dbname", target.name, targetCopy+"/postgres.dump")
	// Restore to fresh runtime roots, leaving the verified transport copy
	// untouched. The private store requires a root owned by the verifier.
	restored := restoreStage + "/restored"
	dualDocker(t, "exec", "--user", "0", target.container, "mkdir", "-m", "700", restored)
	dualDocker(t, "exec", "--user", "0", target.container, "cp", "-R", targetCopy+"/judge", targetCopy+"/private", restored)
	dualDocker(t, "exec", "--user", "0", target.container, "chmod", "700", restored+"/private")
	dsn := "OJ_RESTORED_DATABASE_URL=host=" + restoreSock + " port=5432 user=oj_test dbname=" + target.name + " sslmode=disable"
	if want.checkMissing {
		empty := restored + "/empty-judge"
		dualDocker(t, "exec", "--user", "0", target.container, "mkdir", "-m", "700", empty)
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", "--context", "desktop-linux", "exec", "--user", "0", "-e", dsn,
			target.container, restoreTool, "verify-restored-materials", "-timeout", "8s", target.name, empty, restored+"/private")
		out, err := cmd.CombinedOutput()
		if err == nil || ctx.Err() != nil || !strings.Contains(string(out), "restored judge file ") {
			t.Fatal("restored material check did not explicitly reject missing judge objects")
		}
		t.Log("incomplete restored judge material rejected")
	}
	var checked restorecheck.MaterialSummary
	readRestoreReceipt(t, dualDocker(t, "exec", "--user", "0", "-e", dsn, target.container, restoreTool, "verify-restored-materials", "-timeout", "8s", target.name,
		restored+"/judge", restored+"/private"), &checked)
	if checked != captured.References {
		t.Fatal("restored database material inventory differed from the sealed package")
	}
	// The source submission had finished before capture. This command must be a
	// no-op here; other suites exercise stale and active lease recovery.
	if got := dualDocker(t, "exec", "--user", "0", "-e", dsn, target.container, restoreTool, "fence-restored-leases", target.name); got != "restored leases expired=0 tasks requeued=0" {
		t.Fatal("unexpected in-flight work in the completed source fixture")
	}
	// Feed the controller the judge files from the checked runtime root.
	controllerRoot := filepath.Join(t.TempDir(), "judge")
	dualDocker(t, "cp", target.container+":"+restored+"/judge", controllerRoot)
	t.Logf("verified sealed package %s; PostgreSQL=%s; capture=%s transfer/stop/verify=%s restore/check/stage=%s", trusted, captured.Server,
		captureElapsed, transferElapsed, time.Since(start))
	return controllerRoot
}

func readRestoreReceipt(t *testing.T, data string, result any) {
	t.Helper()
	if err := json.Unmarshal([]byte(data), result); err != nil {
		t.Fatal("restore maintenance command returned an invalid receipt: ", err)
	}
}

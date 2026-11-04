//go:build sandbox

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestSuffixArrayReferenceSandbox(t *testing.T) {
	if os.Getenv("OJ_SUFFIXARRAY_REFERENCE") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv("OJ_WORKER_M0_SLOT") != "" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("explicit independent Desktop suffixarray experiment required")
	}
	referenceSourceCLI(t, "library-checker:suffixarray", "OJ_SUFFIXARRAY_OUTPUT", 50)
}

func referenceSourceCLI(t *testing.T, sourceID, outputVariable string, points int) {
	t.Helper()
	bundle, output := os.Getenv("OJ_REFERENCE_BUNDLE"), os.Getenv(outputVariable)
	if !canonicalAbsolute(bundle) || !canonicalAbsolute(output) {
		t.Fatal("explicit complete bundle/new report directory required")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(binary), "../.."))
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, ".local/validation-host/worker"), "validate-reference", "--source-id", sourceID, "--bundle", bundle, "--source-lock", filepath.Join(root, "config/problem-sources/library-checker.json"), "--output", output, "--start-stopped-slot")
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("real reference CLI: %v: %s", err, raw)
	}
	raw, err = os.ReadFile(filepath.Join(output, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r problemvalidation.Report
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	profile, tier, err := problemvalidation.ResolveSource(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	slot, err := workerhost.ReferenceSlotForTier(tier)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "reference-validation-passed" || r.Verdict != "AC" || !r.StopConfirmed || r.Publication != "blocked" || r.Slot != slot.ContainerName() || r.Plan.SourceID != sourceID || r.Plan.SourceLockSHA256 != problemvalidation.SourceLockSHA256 || len(r.Points) != points || len(r.Plan.Cases) != points || r.ProfileSHA256 != profile.Digest() || r.PlanSHA256 != r.Plan.Digest() {
		t.Fatal("incomplete report identity/profile/execution")
	}
	for _, p := range r.Points {
		if p.Verdict != "AC" || p.Usage == nil {
			t.Fatal("unexecuted reference point")
		}
	}
	if _, err := os.Stat(filepath.Join(output, "INCOMPLETE")); !os.IsNotExist(err) {
		t.Fatal("incomplete report marker")
	}
	lock, err := lockSlotMode(slot.LockName(), false)
	if err != nil {
		t.Fatal("reference lock not clean", err)
	}
	lock.Close()
	usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok || usage.Maxrss <= 0 {
		t.Fatal("native CLI memory counter unavailable")
	}
	// Darwin bytes; this wait4 counter is not Go heap or concurrent total RSS.
	t.Logf("complete source=%s draft=%s import=%s plan=%s profile=%s %d AC/stop confirmed; CLI wait4-maxrss-bytes=%d", sourceID, r.Plan.Version, r.Plan.ImportRevision, r.PlanSHA256, r.ProfileSHA256, points, usage.Maxrss)
}

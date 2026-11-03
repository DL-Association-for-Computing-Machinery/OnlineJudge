package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestReferenceOwnerPersistsExecutionAndStopFailure(t *testing.T) {
	p := validationFixture(t)
	lockPath := filepath.Join(t.TempDir(), "slot.lock")
	lock, err := acquireLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &referenceHost{cancel: cancel}
	h.stopErr = errors.New("PRIVATE-stop")
	report, err := runReferenceLocked(ctx, p, lock, func(context.Context) (validationHost, error) { return h, nil })
	if !errors.Is(err, context.Canceled) || report.Verdict != "SE" || report.StopConfirmed || report.ExecutionDiagnostic == nil || report.StopDiagnostic == nil {
		t.Fatal("Finalize lost execution/stop failure evidence")
	}
	foundRun := false
	for _, e := range report.ExecutionDiagnostic.Entries {
		foundRun = foundRun || e.Operation == "run" && e.Point == 1 && e.Code == "canceled"
	}
	if !foundRun || h.stops != 1 || !h.independent {
		t.Fatal("execution evidence or independent Stop lost")
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "INCOMPLETE"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := persistReferenceReport(directory, report); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(directory, "report.json"))
	if err != nil || strings.Contains(string(raw), "PRIVATE") {
		t.Fatal("failed report leaked raw host error")
	}
	var restored problemvalidation.Report
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.ExecutionDiagnostic, report.ExecutionDiagnostic) || !reflect.DeepEqual(restored.StopDiagnostic, report.StopDiagnostic) || restored.Status != "reference-validation-failed" || restored.Publication != "blocked" {
		t.Fatal("persisted report dropped or accepted failure")
	}
	marker, err := os.ReadFile(lockPath)
	if err != nil || len(marker) == 0 {
		t.Fatal("stop failure cleared quarantine")
	}
}

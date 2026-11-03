package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

type persistedHostEvidence struct{ value experiment.HostFailureTrace }

func (*persistedHostEvidence) Error() string { panic("private host error text must not be read") }
func (*persistedHostEvidence) Unwrap() error { return context.DeadlineExceeded }
func (e *persistedHostEvidence) SandboxHostTrace() (experiment.HostFailureTrace, bool) {
	return e.value, true
}

func persistedEvidence(phase int, elapsed int64) error {
	phases := [...]string{"admission", "prepare", "inspect", "command", "response_validation"}
	raw := experiment.HostFailureTrace{Phase: phases[phase], LastCompleted: phases[phase-1], Count: phase + 1, ElapsedMS: elapsed}
	for i := 0; i <= phase; i++ {
		raw.Steps[i] = experiment.HostFailureStep{Phase: phases[i], Completed: i < phase, ContextBefore: "active", HasDeadline: true, RemainingMS: 1000}
	}
	return &persistedHostEvidence{value: raw}
}

type tracedReferenceHost struct{ referenceHost }

func (h *tracedReferenceHost) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == "/run" && h.requests > 0 {
		return nil, persistedEvidence(3, 10)
	}
	if r.Method == "DELETE" {
		switch r.URL.Path {
		case "/file/upload-2":
			return nil, persistedEvidence(2, 20)
		case "/file/compiled":
			return nil, persistedEvidence(4, 30)
		}
	}
	return h.referenceHost.RoundTrip(r)
}

func TestReferenceOwnerPersistsIndependentHostTraces(t *testing.T) {
	for _, stopFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop-confirmed", true: "stop-failed"}[stopFailure], func(t *testing.T) {
			p := validationFixture(t)
			lockPath := filepath.Join(t.TempDir(), "slot.lock")
			lock, err := acquireLock(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			h := &tracedReferenceHost{}
			if stopFailure {
				h.stopErr = errors.New("PRIVATE-stop")
			}
			report, err := runReferenceLocked(context.Background(), p, lock, func(context.Context) (validationHost, error) { return h, nil })
			if err == nil || report.Verdict != "SE" || report.StopConfirmed == stopFailure || h.stops != 1 || !h.independent || report.ExecutionDiagnostic == nil || len(report.ExecutionDiagnostic.Entries) != 3 {
				t.Fatal("host failures changed owner/Stop or lost diagnostics")
			}
			for i, phase := range []string{"command", "inspect", "response_validation"} {
				e := report.ExecutionDiagnostic.Entries[i]
				point, operation := 1, "cleanup"
				if i == 0 {
					operation = "run"
				}
				if i == 2 {
					point = 0
				}
				if e.HostTrace == nil || e.HostTrace.Phase != phase || e.HostTrace.ElapsedMS != int64((i+1)*10) || e.Operation != operation || e.Point != point {
					t.Fatal("trace assigned to another call or point", i)
				}
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "INCOMPLETE"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			if err := persistReferenceReport(dir, report); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(dir, "report.json"))
			if err != nil || strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "upload-2") {
				t.Fatal("persisted report leaked private host data")
			}
			var restored problemvalidation.Report
			if err := json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(restored.ExecutionDiagnostic, report.ExecutionDiagnostic) || !reflect.DeepEqual(restored.StopDiagnostic, report.StopDiagnostic) || restored.Status != "reference-validation-failed" || restored.Publication != "blocked" {
				t.Fatal("report persistence lost host or Stop evidence")
			}
			marker, err := os.ReadFile(lockPath)
			if err != nil || (len(marker) > 0) != stopFailure {
				t.Fatal("trace changed quarantine cleanup decision")
			}
			info, err := os.Stat(filepath.Join(dir, "report.json"))
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("report permissions changed")
			}
		})
	}
}

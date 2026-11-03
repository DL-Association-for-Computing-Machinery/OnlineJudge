package problemvalidation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

type reportHostEvidence struct{ value experiment.HostFailureTrace }

func (*reportHostEvidence) Error() string { panic("private error must not be formatted") }
func (*reportHostEvidence) Unwrap() error { return context.DeadlineExceeded }
func (e *reportHostEvidence) SandboxHostTrace() (experiment.HostFailureTrace, bool) {
	return e.value, true
}

func TestReportHostTraceIsDetachedAndDoesNotAnnotateContextEntries(t *testing.T) {
	raw := experiment.HostFailureTrace{Phase: "command", LastCompleted: "inspect", Count: 4, ElapsedMS: 10}
	for i, phase := range []string{"admission", "prepare", "inspect", "command"} {
		raw.Steps[i] = experiment.HostFailureStep{Phase: phase, Completed: i < 3, ContextBefore: "active"}
	}
	e := &reportHostEvidence{value: raw}
	projected := execution.FailureDetails(e)
	var report Report
	report.recordExecution("run", 2, e)
	report.recordExecution("validation", 0, context.Canceled)
	e.value.Steps[0].Phase = "PRIVATE"
	projected.Entries[0].HostTrace.Steps[0].Phase = "PRIVATE"
	d := report.ExecutionDiagnostic
	if len(d.Entries) != 2 || d.Entries[0].Operation != "run" || d.Entries[0].Point != 2 || d.Entries[0].HostTrace == nil || d.Entries[0].HostTrace.Steps[0].Phase != "admission" || d.Entries[1].HostTrace != nil {
		t.Fatal("report retained mutable evidence or copied a trace onto context")
	}
	encoded, err := json.Marshal(report)
	if err != nil || strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("report serialized private metadata")
	}
}

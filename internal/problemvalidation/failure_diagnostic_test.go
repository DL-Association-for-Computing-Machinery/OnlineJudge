package problemvalidation

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
)

func TestExecutionDiagnosticAcrossCallsAndBounds(t *testing.T) {
	deleteErr := execution.NewCachedGoJudge(nil, nil, nil).Remove(context.Background(), "foreign")
	r := Report{}
	r.recordExecution("run", 2, errors.Join(context.DeadlineExceeded, deleteErr))
	r.recordExecution("cleanup", 0, context.Canceled)
	want := []ExecutionFailure{
		{Operation: "run", Point: 2, Phase: "unknown", Role: "unknown", Code: "deadline"},
		{Operation: "cleanup", Point: 2, Phase: "delete", Role: "executable", Code: "ownership"},
		{Operation: "cleanup", Phase: "unknown", Role: "unknown", Code: "canceled"},
	}
	if !reflect.DeepEqual(r.ExecutionDiagnostic.Entries, want) || r.ExecutionDiagnostic.Truncated {
		t.Fatalf("call order/point/cleanup mapping: %+v", r.ExecutionDiagnostic)
	}
	for i := 0; i < 5; i++ {
		r.recordExecution("verify", 0, errors.New("PRIVATE"))
	}
	if len(r.ExecutionDiagnostic.Entries) != 8 || r.ExecutionDiagnostic.Truncated {
		t.Fatal("exact eight entries incorrectly truncated")
	}
	r.recordExecution("compile", 0, context.DeadlineExceeded)
	if len(r.ExecutionDiagnostic.Entries) != 8 || !r.ExecutionDiagnostic.Truncated {
		t.Fatal("ninth call did not mark truncation")
	}
	raw, err := json.Marshal(r.ExecutionDiagnostic)
	if err != nil || strings.Contains(string(raw), "PRIVATE") {
		t.Fatal("raw error text serialized")
	}
	var branches []error
	for i := 0; i < 9; i++ {
		branches = append(branches, context.Canceled)
	}
	var capped Report
	capped.recordExecution("compile", 0, errors.Join(branches...))
	if len(capped.ExecutionDiagnostic.Entries) != 8 || !capped.ExecutionDiagnostic.Truncated {
		t.Fatal("adapter truncation not propagated")
	}
	var bad Report
	bad.recordExecution("PRIVATE", 65, context.Canceled)
	if got := bad.ExecutionDiagnostic; !got.Truncated || got.Entries[0].Point != 0 || got.Entries[0].Operation != "unknown" {
		t.Fatal("operation/point projection not bounded")
	}
	var success Report
	success.recordExecution("run", 1, nil)
	if success.ExecutionDiagnostic != nil {
		t.Fatal("success gained diagnostics")
	}
}

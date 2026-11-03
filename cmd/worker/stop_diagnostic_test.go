package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestReferenceStopTraceProjectionIsDetached(t *testing.T) {
	host := &workerhost.StopDiagnostic{Stage: "pid_probe", Code: "deadline", Observed: 128, Confirmed: 127, Probe: 128, ElapsedMS: 60000, ElapsedCapped: true, Steps: []workerhost.StopStep{
		{Stage: "drain", Code: "ok"},
		{Stage: "inspect_before", Code: "ok", ElapsedMS: 1},
		{Stage: "enumerate", Code: "ok", ElapsedMS: 1},
		{Stage: "kill", Code: "ok", ElapsedMS: 1, ContextBefore: "active", CallAttempted: true, LocalProcessStart: "unknown"},
		{Stage: "inspect_after", Code: "ok", ElapsedMS: 1},
		{Stage: "pid_probe", Code: "deadline", ElapsedMS: 60000, ElapsedCapped: true, Count: 128},
	}}
	r := referenceStopDiagnostic(host)
	host.Steps[0].Code = "PRIVATE"
	if r.ElapsedMS != 60000 || !r.ElapsedCapped || len(r.Steps) != 6 || r.Steps[0].Code != "ok" || r.Steps[3].LocalProcessStart != "unknown" || !r.Steps[3].CallAttempted || !r.Steps[5].ElapsedCapped || r.Steps[5].Count != 128 || r.Observed != 128 || r.Confirmed != 127 || r.Probe != 128 {
		t.Fatal("owner dropped or aliased stop evidence")
	}
	raw, err := json.Marshal(r)
	if err != nil || strings.Contains(string(raw), "PRIVATE") || !strings.Contains(string(raw), `"context_before":"active"`) {
		t.Fatal("stop projection changed evidence or leaked state")
	}
	if referenceStopDiagnostic(nil) != nil {
		t.Fatal("successful stop has a diagnostic")
	}
}

package main

import (
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func referenceStopDiagnostic(d *workerhost.StopDiagnostic) *problemvalidation.StopDiagnostic {
	if d == nil {
		return nil
	}
	out := &problemvalidation.StopDiagnostic{Stage: d.Stage, Code: d.Code, Observed: d.Observed, Confirmed: d.Confirmed, Probe: d.Probe, ElapsedMS: d.ElapsedMS, ElapsedCapped: d.ElapsedCapped}
	for _, s := range d.Steps {
		out.Steps = append(out.Steps, problemvalidation.StopStep{Stage: s.Stage, Code: s.Code, ElapsedMS: s.ElapsedMS, ElapsedCapped: s.ElapsedCapped, Count: s.Count, ContextBefore: s.ContextBefore, CallAttempted: s.CallAttempted, LocalProcessStart: s.LocalProcessStart})
	}
	return out
}

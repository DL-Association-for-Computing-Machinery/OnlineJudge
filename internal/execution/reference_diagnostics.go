package execution

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"

func (g *CachedGoJudge) boundDiagnostics(c map[string]any, compile bool) {
	if !g.reference {
		return
	}
	files := c["files"].([]any)
	files[2].(map[string]any)["max"] = experiment.ReferenceDiagnosticBytes
	if compile {
		files[1].(map[string]any)["max"] = experiment.ReferenceDiagnosticBytes
	}
}

func (g *CachedGoJudge) observation(r goJudgeResult, p experiment.ExecutionProfile, compile bool) (Observation, error) {
	if !g.reference {
		return inlineObservation(r, p, compile)
	}
	stdout := p.OutputBytes
	if compile {
		stdout = experiment.ReferenceDiagnosticBytes
	}
	// The pinned executor adds this exact status label when a normally exited
	// process overflows its stdout collector. Admit only the complete, bounded
	// overflow evidence; arbitrary executor errors remain system failures.
	if !compile && referenceCollectorOverflow(r, p) {
		r.Error = ""
	}
	return inlineObservationBounds(r, p, compile, stdout, experiment.ReferenceDiagnosticBytes)
}

func referenceCollectorOverflow(r goJudgeResult, p experiment.ExecutionProfile) bool {
	return r.Error == "Output Limit Exceeded" && r.Status == "Output Limit Exceeded" &&
		r.Exit != nil && *r.Exit == 0 && r.CPU != nil && *r.CPU >= 0 && *r.CPU <= p.Run.CPU &&
		r.Memory != nil && *r.Memory >= 0 && *r.Memory <= p.Run.Memory &&
		len(r.IDs) == 0 && len(r.Files) == 2 && r.Files["stdout"] != nil && len(*r.Files["stdout"]) == p.OutputBytes+1 &&
		r.Files["stderr"] != nil && len(*r.Files["stderr"]) <= 3*experiment.ReferenceDiagnosticBytes &&
		len(r.FileErrors) == 1 && r.FileErrors[0].Name == "stdout" && r.FileErrors[0].Type == "CollectSizeExceeded"
}

// Cached roles were already checked and adopted before this check. A raw
// cap+1 prefix is only meaningful alongside the complete collector receipt.
func checkerTrialOutputEvidence(r goJudgeResult, p experiment.ExecutionProfile) bool {
	if len(r.Files) != 2 || r.Files["stdout"] == nil || r.Files["stderr"] == nil || len(r.IDs) != 0 {
		return false
	}
	if r.Status != "Output Limit Exceeded" {
		return len(*r.Files["stdout"]) <= p.OutputBytes
	}
	if referenceCollectorOverflow(r, p) {
		return true
	}
	return r.Error == "" && len(r.FileErrors) == 0 && r.Exit != nil && *r.Exit == 25 &&
		r.CPU != nil && *r.CPU >= 0 && *r.CPU <= p.Run.CPU &&
		r.Memory != nil && *r.Memory >= 0 && *r.Memory <= p.Run.Memory &&
		len(*r.Files["stdout"]) == p.OutputBytes
}

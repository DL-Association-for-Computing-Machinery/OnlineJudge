package problemvalidation

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"

// ExecutionFailure locates bounded adapter evidence within a validation call.
// Point is one-based for a run and zero outside a point; no material is included.
type ExecutionFailure struct {
	Operation string `json:"operation"`
	Point     int    `json:"point,omitempty"`
	Phase     string `json:"phase"`
	Role      string `json:"role"`
	Code      string `json:"code"`
}

// ExecutionDiagnostic retains primary and cleanup failures in observation order.
type ExecutionDiagnostic struct {
	Entries   []ExecutionFailure `json:"entries"`
	Truncated bool               `json:"truncated,omitempty"`
}

func (r *Report) recordExecution(operation string, point int, err error) {
	d := execution.FailureDetails(err)
	if d == nil {
		return
	}
	if r.ExecutionDiagnostic == nil {
		r.ExecutionDiagnostic = &ExecutionDiagnostic{}
	}
	out := r.ExecutionDiagnostic
	out.Truncated = out.Truncated || d.Truncated
	switch operation {
	case "verify", "compile", "run", "cleanup", "snapshot_close", "validation":
	default:
		operation = "unknown"
	}
	if point < 0 || point > 64 {
		point = 0
		out.Truncated = true
	}
	for _, e := range d.Entries {
		if len(out.Entries) == 8 {
			out.Truncated = true
			break
		}
		op := operation
		if e.Phase == "delete" {
			op = "cleanup"
		}
		out.Entries = append(out.Entries, ExecutionFailure{Operation: op, Point: point, Phase: e.Phase, Role: e.Role, Code: e.Code})
	}
}

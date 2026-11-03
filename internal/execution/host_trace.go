package execution

import (
	"context"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

// HostTrace locates a failed control operation without command or object data.
// Stage durations exclude deferred cleanup; total elapsed includes that cleanup.
type HostTrace struct {
	Phase         string     `json:"phase"`
	LastCompleted string     `json:"last_completed"`
	ElapsedMS     int64      `json:"elapsed_ms"`
	ElapsedCapped bool       `json:"elapsed_capped,omitempty"`
	Steps         []HostStep `json:"steps"`
}

// HostStep describes one reached phase. Process startup concerns only the local
// CLI, never daemon receipt. A rounded zero remaining time does not prove expiry.
type HostStep struct {
	Phase             string `json:"phase"`
	Completed         bool   `json:"completed"`
	ElapsedMS         int64  `json:"elapsed_ms"`
	ElapsedCapped     bool   `json:"elapsed_capped,omitempty"`
	ContextBefore     string `json:"context_before"`
	HasDeadline       bool   `json:"has_deadline"`
	RemainingMS       int64  `json:"remaining_ms"`
	RemainingCapped   bool   `json:"remaining_capped,omitempty"`
	CallAttempted     bool   `json:"call_attempted,omitempty"`
	LocalProcessStart string `json:"local_process_start,omitempty"`
}

type hostTraceProvider interface {
	SandboxHostTrace() (experiment.HostFailureTrace, bool)
}

func boundedTraceMS(value int64, capped bool) (int64, bool) {
	if value < 0 {
		return 0, true
	}
	if value > 60000 {
		return 60000, true
	}
	return value, capped
}

func safeHostContext(value string) string {
	switch value {
	case "active":
		return "active"
	case "canceled":
		return "canceled"
	case "deadline":
		return "deadline"
	default:
		return "unknown"
	}
}

func safeProcessStart(value string) string {
	switch value {
	case "started":
		return "started"
	case "not_started":
		return "not_started"
	default:
		return "unknown"
	}
}

func sanitizeHostTrace(raw experiment.HostFailureTrace) (experiment.HostFailureTrace, bool) {
	phases := [...]string{"admission", "prepare", "inspect", "command", "response_validation"}
	var clean experiment.HostFailureTrace
	if raw.Count < 1 || raw.Count > len(phases) || raw.Phase != phases[raw.Count-1] {
		return clean, false
	}
	clean.Count, clean.Phase, clean.LastCompleted = raw.Count, phases[raw.Count-1], "none"
	if raw.Count > 1 {
		clean.LastCompleted = phases[raw.Count-2]
	}
	if raw.LastCompleted != clean.LastCompleted {
		return experiment.HostFailureTrace{}, false
	}
	clean.ElapsedMS, clean.ElapsedCapped = boundedTraceMS(raw.ElapsedMS, raw.ElapsedCapped)
	for i := 0; i < raw.Count; i++ {
		s := raw.Steps[i]
		if s.Phase != phases[i] || s.Completed != (i < raw.Count-1) {
			return experiment.HostFailureTrace{}, false
		}
		out := &clean.Steps[i]
		out.Phase, out.Completed = phases[i], s.Completed
		out.ElapsedMS, out.ElapsedCapped = boundedTraceMS(s.ElapsedMS, s.ElapsedCapped)
		out.ContextBefore = safeHostContext(s.ContextBefore)
		out.HasDeadline = s.HasDeadline
		if s.HasDeadline {
			out.RemainingMS, out.RemainingCapped = boundedTraceMS(s.RemainingMS, s.RemainingCapped)
		}
		if i == 2 || i == 3 {
			out.CallAttempted = s.CallAttempted
			out.LocalProcessStart = "unknown"
			if s.CallAttempted {
				out.LocalProcessStart = safeProcessStart(s.LocalProcessStart)
			}
		}
	}
	return clean, true
}

// Only a unique trace can describe one tagged adapter call. Do not borrow the
// first trace from multiple independent joined operations.
func hostTraceFrom(err error) (experiment.HostFailureTrace, bool) {
	var trace experiment.HostFailureTrace
	count := 0
	ambiguous := false
	truncated := walkFailureBranches(err, func(current error) bool {
		if current == context.Canceled || current == context.DeadlineExceeded {
			return false
		}
		if tagged, ok := current.(*executionFailure); ok {
			if value, valid := hostTraceValue(tagged.detail.HostTrace); valid {
				trace = value
				count++
			} else {
				ambiguous = true
			}
			return false
		}
		if provider, ok := current.(hostTraceProvider); ok {
			if value, present := provider.SandboxHostTrace(); present {
				if clean, valid := sanitizeHostTrace(value); valid {
					trace = clean
					count++
				} else {
					ambiguous = true
				}
				return false
			}
		}
		return true
	}, func() { ambiguous = true })
	if count != 1 || truncated || ambiguous {
		return experiment.HostFailureTrace{}, false
	}
	return trace, true
}

func projectHostTrace(raw experiment.HostFailureTrace) *HostTrace {
	clean, valid := sanitizeHostTrace(raw)
	if !valid {
		return nil
	}
	d := &HostTrace{Phase: clean.Phase, LastCompleted: clean.LastCompleted, ElapsedMS: clean.ElapsedMS, ElapsedCapped: clean.ElapsedCapped, Steps: make([]HostStep, clean.Count)}
	for i := range d.Steps {
		s := clean.Steps[i]
		d.Steps[i] = HostStep{Phase: s.Phase, Completed: s.Completed, ElapsedMS: s.ElapsedMS, ElapsedCapped: s.ElapsedCapped, ContextBefore: s.ContextBefore, HasDeadline: s.HasDeadline, RemainingMS: s.RemainingMS, RemainingCapped: s.RemainingCapped, CallAttempted: s.CallAttempted, LocalProcessStart: s.LocalProcessStart}
	}
	return d
}

func hostTraceDetails(err error) *HostTrace {
	if value, ok := hostTraceFrom(err); ok {
		return projectHostTrace(value)
	}
	return nil
}

// CloneHostTrace returns a fresh validated report value. Missing or malformed
// evidence stays absent; it does not establish that no host phase was reached.
func CloneHostTrace(trace *HostTrace) *HostTrace {
	raw, valid := hostTraceValue(trace)
	if !valid {
		return nil
	}
	return projectHostTrace(raw)
}

func hostTraceValue(trace *HostTrace) (experiment.HostFailureTrace, bool) {
	if trace == nil || len(trace.Steps) < 1 || len(trace.Steps) > 5 {
		return experiment.HostFailureTrace{}, false
	}
	raw := experiment.HostFailureTrace{Phase: trace.Phase, LastCompleted: trace.LastCompleted, ElapsedMS: trace.ElapsedMS, ElapsedCapped: trace.ElapsedCapped, Count: len(trace.Steps)}
	for i, s := range trace.Steps {
		raw.Steps[i] = experiment.HostFailureStep{Phase: s.Phase, Completed: s.Completed, ElapsedMS: s.ElapsedMS, ElapsedCapped: s.ElapsedCapped, ContextBefore: s.ContextBefore, HasDeadline: s.HasDeadline, RemainingMS: s.RemainingMS, RemainingCapped: s.RemainingCapped, CallAttempted: s.CallAttempted, LocalProcessStart: s.LocalProcessStart}
	}
	return sanitizeHostTrace(raw)
}

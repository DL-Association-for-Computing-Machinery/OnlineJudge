package workerhost

import (
	"context"
	"errors"
)

// StopDiagnostic is a bounded projection of stop evidence, never a stop proof.
// Counts contain no PID values; Probe is one-based and zero outside a probe.
type StopDiagnostic struct {
	Stage         string     `json:"stage"`
	Code          string     `json:"code"`
	Observed      int        `json:"observed"`
	Confirmed     int        `json:"confirmed"`
	Probe         int        `json:"probe,omitempty"`
	ElapsedMS     int64      `json:"elapsed_ms,omitempty"`
	ElapsedCapped bool       `json:"elapsed_capped,omitempty"`
	Steps         []StopStep `json:"steps,omitempty"`
}

type stopFailure struct {
	detail StopDiagnostic
	cause  error
}

func (e *stopFailure) Error() string {
	return "sandbox stop unconfirmed: " + e.detail.Stage + "/" + e.detail.Code
}
func (e *stopFailure) Unwrap() error { return e.cause }

// StopDetails returns only diagnostics produced by this adapter. Unknown errors
// never contribute text, command arguments, raw exit codes or process IDs.
func StopDetails(err error) *StopDiagnostic {
	if err == nil {
		return nil
	}
	var failure *stopFailure
	if errors.As(err, &failure) {
		copy := failure.detail
		copy.Steps = append([]StopStep(nil), failure.detail.Steps...)
		return &copy
	}
	return &StopDiagnostic{Stage: "unknown", Code: failureCode(err)}
}

func failureCode(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var command *commandFailure
	if errors.As(err, &command) {
		return projectedFailureCode(command.code)
	}
	return "unknown"
}

func stopError(stage, code string, observed, confirmed, probe int, err error) error {
	return &stopFailure{detail: StopDiagnostic{Stage: stage, Code: code, Observed: observed, Confirmed: confirmed, Probe: probe}, cause: contextCause(err)}
}

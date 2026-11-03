package execution

import (
	"context"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

// FailureEntry identifies a trusted adapter call site, never command or file data.
type FailureEntry struct {
	Phase     string     `json:"phase"`
	Role      string     `json:"role"`
	Code      string     `json:"code"`
	HostTrace *HostTrace `json:"host_trace,omitempty"`
}

// FailureDiagnostic bounds joined primary and cleanup evidence to eight entries.
type FailureDiagnostic struct {
	Entries   []FailureEntry `json:"entries"`
	Truncated bool           `json:"truncated,omitempty"`
}

type executionFailure struct {
	detail FailureEntry
	cause  error
}

func (e *executionFailure) Error() string { return e.cause.Error() }
func (e *executionFailure) Unwrap() error { return e.cause }

type codedFailure struct {
	code    string
	cause   error
	host    experiment.HostFailureTrace
	hasHost bool
}

func (e *codedFailure) Error() string              { return e.cause.Error() }
func (e *codedFailure) Unwrap() error              { return e.cause }
func (e *codedFailure) SandboxFailureCode() string { return safeFailureCode(e.code) }

func (e *codedFailure) SandboxHostTrace() (experiment.HostFailureTrace, bool) {
	return e.host, e.hasHost
}

func withFailureCode(err error, code string) error {
	if err == nil {
		return nil
	}
	return &codedFailure{code: safeFailureCode(code), cause: err}
}

func withFailureMetadata(label error, original error) error {
	if label == nil {
		return nil
	}
	trace, present := hostTraceFrom(original)
	return &codedFailure{code: errorFailureCode(original), cause: label, host: trace, hasHost: present}
}

func safeFailureCode(code string) string {
	switch code {
	case "deadline", "canceled", "budget", "integrity", "ownership", "invalid_evidence", "http_rejected", "decode", "command_exit", "launch_failed", "output_limit", "wait_delay":
		return code
	default:
		return "unknown"
	}
}

func nodeFailureCode(err error) string {
	if err == context.DeadlineExceeded {
		return "deadline"
	}
	if err == context.Canceled {
		return "canceled"
	}
	if err == ErrBusy {
		return "ownership"
	}
	if c, ok := err.(interface{ SandboxFailureCode() string }); ok {
		return safeFailureCode(c.SandboxFailureCode())
	}
	return "unknown"
}

// Count every inspected slot, including nil children, to bound wide graphs as
// well as cycles. Calling foreign Unwrap itself is outside this traversal bound.
// errors.As at each node would defeat the budget and collapse cleanup errors.
func walkFailures(err error, visit func(error) bool) (truncated bool) {
	return walkFailureBranches(err, visit, func() {})
}

func walkFailureBranches(err error, visit func(error) bool, leaf func()) (truncated bool) {
	slots := 0
	var walk func(error)
	walk = func(current error) {
		if slots >= 64 {
			truncated = true
			return
		}
		slots++
		if current == nil || !visit(current) {
			return
		}
		switch e := current.(type) {
		case interface{ Unwrap() []error }:
			children := e.Unwrap()
			hasChild := false
			for _, child := range children {
				if slots >= 64 {
					truncated = true
					break
				}
				hasChild = hasChild || child != nil
				walk(child)
			}
			if !hasChild && !truncated {
				leaf()
			}
		case interface{ Unwrap() error }:
			child := e.Unwrap()
			walk(child)
			if child == nil && !truncated {
				leaf()
			}
		default:
			leaf()
		}
	}
	walk(err)
	return truncated
}

func errorFailureCode(err error) string {
	code := "unknown"
	walkFailures(err, func(current error) bool {
		if code == "unknown" {
			code = nodeFailureCode(current)
		}
		return code == "unknown"
	})
	return code
}

func tagFailure(err error, phase, role, code string) error {
	if err == nil {
		return nil
	}
	if code == "" {
		code = errorFailureCode(err)
	}
	return &executionFailure{detail: FailureEntry{Phase: phase, Role: role, Code: safeFailureCode(code), HostTrace: hostTraceDetails(err)}, cause: err}
}

func failureError(phase, role, code, label string) error {
	return tagFailure(errors.New(label), phase, role, code)
}

func safeFailureEntry(d FailureEntry) FailureEntry {
	switch d.Phase {
	case "verify", "upload", "readback", "control", "stdout_read", "validate", "delete":
	default:
		d.Phase = "unknown"
	}
	switch d.Role {
	case "source", "dependency", "input", "executable", "stdout", "environment":
	default:
		d.Role = "unknown"
	}
	d.Code = safeFailureCode(d.Code)
	d.HostTrace = CloneHostTrace(d.HostTrace)
	return d
}

// FailureDetails returns fresh, whitelisted evidence without raw error text.
// Unknown adapters remain unknown; a control error is not proof of execution.
func FailureDetails(err error) *FailureDiagnostic {
	if err == nil {
		return nil
	}
	d := &FailureDiagnostic{}
	appendEntry := func(entry FailureEntry) {
		if len(d.Entries) == 8 {
			d.Truncated = true
		} else {
			d.Entries = append(d.Entries, safeFailureEntry(entry))
		}
	}
	d.Truncated = walkFailureBranches(err, func(current error) bool {
		if e, ok := current.(*executionFailure); ok {
			appendEntry(e.detail)
			return false
		}
		if provider, ok := current.(hostTraceProvider); ok {
			if value, present := provider.SandboxHostTrace(); present {
				if trace := projectHostTrace(value); trace != nil {
					appendEntry(FailureEntry{Phase: "unknown", Role: "unknown", Code: nodeFailureCode(current), HostTrace: trace})
					return false
				}
			}
		}
		if code := nodeFailureCode(current); code != "unknown" {
			appendEntry(FailureEntry{Phase: "unknown", Role: "unknown", Code: code})
			return false
		}
		return true
	}, func() {
		appendEntry(FailureEntry{Phase: "unknown", Role: "unknown", Code: "unknown"})
	}) || d.Truncated
	if len(d.Entries) == 0 {
		d.Entries = []FailureEntry{{Phase: "unknown", Role: "unknown", Code: "unknown"}}
	}
	return d
}

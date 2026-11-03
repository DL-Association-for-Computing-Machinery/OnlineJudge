package workerhost

type fileFailure struct{ label, code string }

func (e *fileFailure) Error() string { return e.label }

// SandboxFailureCode exposes only a fixed category to the execution adapter.
func (e *fileFailure) SandboxFailureCode() string {
	switch e.code {
	case "budget", "ownership", "decode", "http_rejected":
		return e.code
	default:
		return projectedFailureCode(e.code)
	}
}

// SandboxFailureCode excludes command arguments, stderr and exit codes.
func (e *commandFailure) SandboxFailureCode() string { return projectedFailureCode(e.code) }

func fileError(label, code string) error {
	return &fileFailure{label: label, code: code}
}

func projectedFailureCode(code string) string {
	switch code {
	case "deadline", "canceled", "command_exit", "launch_failed", "output_limit", "wait_delay":
		return code
	default:
		return "unknown"
	}
}

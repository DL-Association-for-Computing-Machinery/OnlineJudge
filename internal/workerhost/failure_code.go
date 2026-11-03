package workerhost

func projectedFailureCode(code string) string {
	switch code {
	case "deadline", "canceled", "command_exit", "launch_failed", "output_limit", "wait_delay":
		return code
	default:
		return "unknown"
	}
}

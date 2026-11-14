package execution

import (
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
)

// verifyCheckerResult interprets a checker receipt only after verifying that
// the executor observed a normal exit within the checker-specific budget.
// Checker diagnostics never participate in this decision.
func verifyCheckerResult(r goJudgeResult, protocol judging.CheckerProtocol, limits experiment.Limits, diagnosticBytes int) (judging.CheckerOutcome, error) {
	if limits.CPU <= 0 || limits.Memory <= 0 || diagnosticBytes <= 0 ||
		r.Exit == nil || r.CPU == nil || r.Memory == nil || r.Error != "" ||
		r.Status != "Nonzero Exit Status" || *r.Exit <= 0 || *r.Exit > 255 ||
		*r.CPU < 0 || *r.CPU > limits.CPU || *r.Memory < 0 || *r.Memory > limits.Memory ||
		len(r.FileErrors) != 0 || len(r.IDs) != 0 || len(r.Files) != 2 {
		return judging.CheckerSystemError, judging.ErrCheckerResult
	}
	for _, name := range []string{"stdout", "stderr"} {
		// Keep this bound on decoded bytes. A malformed diagnostic that expands
		// during JSON transport may fail closed.
		if stream := r.Files[name]; stream == nil || len(*stream) > diagnosticBytes {
			return judging.CheckerSystemError, judging.ErrCheckerResult
		}
	}
	return judging.DecodeCheckerExit(protocol, true, *r.Exit)
}

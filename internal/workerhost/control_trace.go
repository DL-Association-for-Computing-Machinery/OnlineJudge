package workerhost

import (
	"context"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

type controlTrace struct {
	started, stepStarted time.Time
	detail               experiment.HostFailureTrace
	frozen               bool
}

type controlFailure struct {
	cause  error
	detail experiment.HostFailureTrace
}

func (e *controlFailure) Error() string { return e.cause.Error() }
func (e *controlFailure) Unwrap() error { return e.cause }

// SandboxHostTrace returns detached evidence; a caller cannot mutate the error.
func (e *controlFailure) SandboxHostTrace() (experiment.HostFailureTrace, bool) {
	return e.detail, true
}

func newControlTrace(ctx context.Context) *controlTrace {
	t := &controlTrace{started: time.Now()}
	t.detail.LastCompleted = "none"
	t.enter(ctx, "admission")
	return t
}

func (t *controlTrace) enter(ctx context.Context, phase string) {
	if t.detail.Count == len(t.detail.Steps) {
		return
	}
	if t.detail.Count != 0 {
		previous := &t.detail.Steps[t.detail.Count-1]
		previous.Completed = true
		previous.ElapsedMS, previous.ElapsedCapped = boundedElapsed(t.stepStarted)
		t.detail.LastCompleted = previous.Phase
	}
	t.stepStarted = time.Now()
	t.detail.Phase = phase
	step := &t.detail.Steps[t.detail.Count]
	t.detail.Count++
	step.Phase, step.ContextBefore = phase, "unknown"
	if ctx != nil {
		step.ContextBefore = contextState(ctx)
		if deadline, ok := ctx.Deadline(); ok {
			step.HasDeadline = true
			remaining := time.Until(deadline)
			switch {
			case remaining < 0:
				step.RemainingCapped = true
			case remaining > time.Minute:
				step.RemainingMS, step.RemainingCapped = 60000, true
			default:
				step.RemainingMS = remaining.Milliseconds()
			}
		}
	}
}

func (t *controlTrace) attempt() {
	step := &t.detail.Steps[t.detail.Count-1]
	step.CallAttempted, step.LocalProcessStart = true, "unknown"
}

func (t *controlTrace) commandResult(err error) {
	// The real command executor returns this concrete local error directly.
	// Wrapped or substitute errors cannot establish that a process started.
	if command, ok := err.(*commandFailure); ok {
		switch command.processStart {
		case "started", "not_started":
			t.detail.Steps[t.detail.Count-1].LocalProcessStart = command.processStart
		}
	}
}

func (t *controlTrace) attach(err error) error {
	if err == nil {
		return nil
	}
	err = t.freeze(err)
	t.detail.ElapsedMS, t.detail.ElapsedCapped = boundedElapsed(t.started)
	return &controlFailure{cause: err, detail: t.detail}
}

func (t *controlTrace) freeze(err error) error {
	if err != nil && !t.frozen {
		// Capture the failed phase before deferred body/Host cleanup. Overall
		// operation time is separately sampled after that cleanup has finished.
		step := &t.detail.Steps[t.detail.Count-1]
		step.ElapsedMS, step.ElapsedCapped = boundedElapsed(t.stepStarted)
		t.frozen = true
	}
	return err
}

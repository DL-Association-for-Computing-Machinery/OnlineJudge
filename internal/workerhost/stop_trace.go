package workerhost

import (
	"context"
	"errors"
	"time"
)

// StopStep records a reached stop step. LocalProcessStart concerns only the
// local Docker CLI, never daemon receipt or delivery of a container signal.
type StopStep struct {
	Stage             string `json:"stage"`
	Code              string `json:"code"`
	ElapsedMS         int64  `json:"elapsed_ms"`
	ElapsedCapped     bool   `json:"elapsed_capped,omitempty"`
	Count             int    `json:"count,omitempty"`
	ContextBefore     string `json:"context_before,omitempty"`
	CallAttempted     bool   `json:"call_attempted,omitempty"`
	LocalProcessStart string `json:"local_process_start,omitempty"`
}

type stopTrace struct {
	started time.Time
	steps   [6]StopStep
	n       int
}

func boundedElapsed(start time.Time) (int64, bool) {
	d := time.Since(start)
	if d < 0 {
		return 0, true
	}
	if d > time.Minute {
		return 60000, true
	}
	return d.Milliseconds(), false
}

func (t *stopTrace) record(stage string, start time.Time, code string) *StopStep {
	if t.n >= len(t.steps) {
		return nil
	}
	step := &t.steps[t.n]
	t.n++
	step.Stage, step.Code = stage, code
	step.ElapsedMS, step.ElapsedCapped = boundedElapsed(start)
	return step
}

func stepCode(err error) string {
	if err == nil {
		return "ok"
	}
	return failureCode(err)
}

func contextState(ctx context.Context) string {
	if ctx.Err() == nil {
		return "active"
	}
	return failureCode(ctx.Err())
}

func (t *stopTrace) recordKill(start time.Time, before string, err error) {
	step := t.record("kill", start, stepCode(err))
	if step == nil {
		return
	}
	step.ContextBefore, step.CallAttempted, step.LocalProcessStart = before, true, "unknown"
	var command *commandFailure
	if errors.As(err, &command) {
		switch command.processStart {
		case "started", "not_started":
			step.LocalProcessStart = command.processStart
		}
	}
}

func (t *stopTrace) attach(err error) error {
	if err == nil {
		return nil
	}
	var failure *stopFailure
	if !errors.As(err, &failure) {
		return err
	}
	copy := *failure
	copy.detail.Steps = append([]StopStep(nil), t.steps[:t.n]...)
	copy.detail.ElapsedMS, copy.detail.ElapsedCapped = boundedElapsed(t.started)
	return &copy
}

package workerhost

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Stop permanently fences this adapter, cancels and drains its local requests,
// kills the bound container lifetime, and verifies stopped state plus absence of
// the host PIDs observed before the kill. PID reuse conservatively fails. It does
// not restart the container, and must be called with an independent context.
func (h *Host) Stop(ctx context.Context) (result error) {
	trace := stopTrace{started: time.Now()}
	defer func() { result = trace.attach(result) }()
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return stopError("fence", "already_fenced", 0, 0, 0, nil)
	}
	h.closed = true
	for _, cancel := range h.active {
		cancel()
	}
	if len(h.active) == 0 {
		close(h.drained)
	}
	h.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Drain client dispatch before killing: no delayed docker exec may race a
	// successful stop acknowledgement. Closing the gate precedes cancellation.
	stepStart := time.Now()
	select {
	case <-h.drained:
		trace.record("drain", stepStart, "ok")
	case <-ctx.Done():
		trace.record("drain", stepStart, failureCode(ctx.Err()))
		return stopError("drain", failureCode(ctx.Err()), 0, 0, 0, ctx.Err())
	}
	stepStart = time.Now()
	s, err := h.inspect(ctx, h.id)
	if err != nil {
		trace.record("inspect_before", stepStart, failureCode(err))
		return stopError("inspect_before", failureCode(err), 0, 0, 0, err)
	}
	if s.ID != h.id || s.Started != h.started || s.Restart != "no" || s.Restarting {
		trace.record("inspect_before", stepStart, "lifetime_changed")
		return stopError("inspect_before", "lifetime_changed", 0, 0, 0, nil)
	}
	if !s.Running || s.PID <= 1 {
		trace.record("inspect_before", stepStart, "evidence_unavailable")
		return stopError("inspect_before", "evidence_unavailable", 0, 0, 0, nil)
	}
	trace.record("inspect_before", stepStart, "ok")
	stepStart = time.Now()
	enumerateCtx, finishEnumeration := stopPhaseContext(ctx)
	out, topErr := h.docker(enumerateCtx, nil, "top", h.id, "-eo", "pid")
	finishEnumeration()
	pids, parseErr := parsePIDs(string(out), s.PID)
	code := stepCode(topErr)
	if topErr == nil && parseErr != nil {
		code = "invalid_evidence"
	}
	trace.record("enumerate", stepStart, code)
	// Even if process enumeration fails, kill the verified owned lifetime. Missing
	// evidence must never become success, but should not leave known work running.
	stepStart = time.Now()
	killCtx, finishKill := stopPhaseContext(ctx)
	beforeKill := contextState(killCtx)
	_, killErr := h.docker(killCtx, nil, "kill", "--signal", "KILL", h.id)
	finishKill()
	trace.recordKill(stepStart, beforeKill, killErr)
	// A failed kill takes diagnostic precedence over missing enumeration; neither
	// outcome can become a stop acknowledgement. Enumeration always precedes kill.
	if killErr != nil {
		return stopError("kill", failureCode(killErr), len(pids), 0, 0, errors.Join(topErr, killErr))
	}
	if topErr != nil {
		return stopError("enumerate", failureCode(topErr), 0, 0, 0, topErr)
	}
	if parseErr != nil {
		return stopError("enumerate", "invalid_evidence", 0, 0, 0, nil)
	}
	stepStart = time.Now()
	s, err = h.inspect(ctx, h.id)
	if err != nil {
		trace.record("inspect_after", stepStart, failureCode(err))
		return stopError("inspect_after", failureCode(err), len(pids), 0, 0, err)
	}
	if s.ID != h.id || s.Started != h.started || s.Running || s.PID != 0 || s.Restarting {
		trace.record("inspect_after", stepStart, "state_unconfirmed")
		return stopError("inspect_after", "state_unconfirmed", len(pids), 0, 0, nil)
	}
	trace.record("inspect_after", stepStart, "ok")
	stepStart = time.Now()
	for i, pid := range pids {
		_, err := h.run(ctx, nil, "docker", PIDProbeArgs(strconv.Itoa(pid))...)
		if err != nil {
			code := failureCode(err)
			var command *commandFailure
			if code == "command_exit" && errors.As(err, &command) {
				switch command.exit {
				case probeProcUnavailable:
					code = "proc_unavailable"
				case probePIDPresent:
					code = "pid_present"
				}
			}
			if step := trace.record("pid_probe", stepStart, code); step != nil {
				step.Count = i + 1
			}
			return stopError("pid_probe", code, len(pids), i, i+1, err)
		}
	}
	if step := trace.record("pid_probe", stepStart, "ok"); step != nil {
		step.Count = len(pids)
	}
	if err := ctx.Err(); err != nil {
		return stopError("complete", failureCode(err), len(pids), len(pids), 0, err)
	}
	return nil
}

// Preserve time for termination after enumeration and for evidence after kill.
// This bounds scheduled command work, not process teardown or OS scheduling.
func stopPhaseContext(parent context.Context) (context.Context, context.CancelFunc) {
	const budget = 2 * time.Second
	now := time.Now()
	phaseDeadline := now.Add(budget)
	if deadline, ok := parent.Deadline(); ok {
		remaining := deadline.Sub(now)
		if remaining <= 0 {
			// A later deadline would depend on parent cancellation reaching
			// this child, even though the parent's time is already exhausted.
			phaseDeadline = deadline
		} else {
			phaseDeadline = now.Add(min(budget, remaining/2))
		}
	}
	return context.WithDeadline(parent, phaseDeadline)
}

func parsePIDs(out string, initPID int) ([]int, error) {
	fields := strings.Fields(out)
	if len(fields) < 2 || len(fields) > 129 || fields[0] != "PID" {
		return nil, errors.New("invalid sandbox process evidence")
	}
	seen := map[int]bool{}
	var pids []int
	for _, f := range fields[1:] {
		pid, err := strconv.Atoi(f)
		if err != nil || pid <= 1 || seen[pid] {
			return nil, errors.New("invalid sandbox PID")
		}
		seen[pid] = true
		pids = append(pids, pid)
	}
	if !seen[initPID] {
		return nil, errors.New("sandbox init PID missing")
	}
	return pids, nil
}

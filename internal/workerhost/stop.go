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
func (h *Host) Stop(ctx context.Context) error {
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
	select {
	case <-h.drained:
	case <-ctx.Done():
		return stopError("drain", failureCode(ctx.Err()), 0, 0, 0, ctx.Err())
	}
	s, err := h.inspect(ctx, h.id)
	if err != nil {
		return stopError("inspect_before", failureCode(err), 0, 0, 0, err)
	}
	if s.ID != h.id || s.Started != h.started || s.Restart != "no" || s.Restarting {
		return stopError("inspect_before", "lifetime_changed", 0, 0, 0, nil)
	}
	if !s.Running || s.PID <= 1 {
		return stopError("inspect_before", "evidence_unavailable", 0, 0, 0, nil)
	}
	out, topErr := h.docker(ctx, nil, "top", h.id, "-eo", "pid")
	pids, parseErr := parsePIDs(string(out), s.PID)
	// Even if process enumeration fails, kill the verified owned lifetime. Missing
	// evidence must never become success, but should not leave known work running.
	_, killErr := h.docker(ctx, nil, "kill", "--signal", "KILL", h.id)
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
	s, err = h.inspect(ctx, h.id)
	if err != nil {
		return stopError("inspect_after", failureCode(err), len(pids), 0, 0, err)
	}
	if s.ID != h.id || s.Started != h.started || s.Running || s.PID != 0 || s.Restarting {
		return stopError("inspect_after", "state_unconfirmed", len(pids), 0, 0, nil)
	}
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
			return stopError("pid_probe", code, len(pids), i, i+1, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return stopError("complete", failureCode(err), len(pids), len(pids), 0, err)
	}
	return nil
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

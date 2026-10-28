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
		return errors.New("sandbox already fenced")
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
		return ctx.Err()
	}
	s, err := h.inspect(ctx, h.id)
	if err != nil {
		return err
	}
	if s.ID != h.id || s.Started != h.started || s.Restart != "no" || s.Restarting {
		return errors.New("sandbox lifetime changed; stop unconfirmed")
	}
	if !s.Running || s.PID <= 1 {
		return errors.New("sandbox process evidence unavailable")
	}
	out, topErr := h.docker(ctx, nil, "top", h.id, "-eo", "pid")
	pids, parseErr := parsePIDs(string(out), s.PID)
	// Even if process enumeration fails, kill the verified owned lifetime. Missing
	// evidence must never become success, but should not leave known work running.
	_, killErr := h.docker(ctx, nil, "kill", "--signal", "KILL", h.id)
	if err := errors.Join(topErr, parseErr, killErr); err != nil {
		return err
	}
	s, err = h.inspect(ctx, h.id)
	if err != nil || s.ID != h.id || s.Started != h.started || s.Running || s.PID != 0 || s.Restarting {
		return errors.New("sandbox stopped state unconfirmed")
	}
	for _, pid := range pids {
		_, err := h.run(ctx, nil, "colima", "--profile", "oj-m0", "ssh", "--", "test", "!", "-e", "/proc/"+strconv.Itoa(pid))
		if err != nil {
			return errors.New("sandbox process exit unconfirmed")
		}
	}
	return ctx.Err()
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

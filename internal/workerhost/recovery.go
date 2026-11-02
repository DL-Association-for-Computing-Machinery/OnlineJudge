package workerhost

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

// RecoverM0Slot confirms termination of a quarantined slot. The caller must hold
// its canonical lock, exclusively own the trusted Docker control plane, confirm
// all old worker host CLI processes and in-flight control operations have drained,
// and supply the original container ID from a trusted pre-crash record. This
// operation cannot establish that cross-process dispatch fence itself. The marker
// itself does not record identity. Unknown or replaced containers require manual
// investigation. This operation never starts a container or claims work.
func RecoverM0Slot(ctx context.Context, slot M0Slot, expectedID string) error {
	return recoverSlot(ctx, runCommand, slot, expectedID)
}

func recoverSlot(ctx context.Context, run command, slot M0Slot, expectedID string) error {
	if !containerID.MatchString(expectedID) {
		return errors.New("original full M0 container ID required")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	h := &Host{run: run, id: expectedID, active: make(map[uint64]context.CancelFunc), drained: make(chan struct{})}
	s, err := h.inspect(ctx, slot.ContainerName())
	if err != nil {
		return errors.New("recovery sandbox inspection failed")
	}
	if s.ID != expectedID || s.Started == "" || s.Restarting || s.Paused {
		return errors.New("recovery sandbox identity or lifetime unavailable")
	}
	if err := validateIsolation(s, experiment.PinnedEnvironment()); err != nil {
		return err
	}
	h.started = s.Started
	if s.Running {
		// Keep the ordinary Stop contract, including observed PID disappearance.
		if err := h.Stop(ctx); err != nil {
			return errors.New("recovery sandbox termination unconfirmed")
		}
	} else if s.PID != 0 {
		return errors.New("recovery sandbox stopped state inconsistent")
	}
	// Even an already stopped container needs a fresh readback by immutable ID.
	// No REST/compiler probe runs while recovering a potentially unhealthy host.
	s, err = h.inspect(ctx, expectedID)
	if err != nil || s.ID != expectedID || s.Started != h.started || s.Running || s.PID != 0 || s.Restarting || s.Paused {
		return errors.New("recovery sandbox stopped state unconfirmed")
	}
	if err := validateIsolation(s, experiment.PinnedEnvironment()); err != nil {
		return err
	}
	return ctx.Err()
}

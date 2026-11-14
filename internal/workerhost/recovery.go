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
	return recoverConfiguredSlot(ctx, run, slot.ContainerName(), expectedID, 0)
}

// RecoverReference confirms only the fixed reference slot's original lifetime.
// It has the same external dispatch-fence prerequisites as RecoverM0Slot and
// never starts a container or clears a marker itself.
func RecoverReference(ctx context.Context, expectedID string) error {
	return recoverConfiguredSlot(ctx, runCommand, ReferenceSlot{}.ContainerName(), expectedID, experiment.ReferenceSmall)
}

// RecoverReferenceSlot confirms a fixed tier slot under the same external
// dispatch-fence prerequisites as RecoverM0Slot. It never starts the container.
func RecoverReferenceSlot(ctx context.Context, slot ReferenceTierSlot, expectedID string) error {
	name, _, tier, err := slot.configuration()
	if err != nil {
		return err
	}
	if slot.IsUbuntuAPlusB() {
		h := &Host{run: ubuntuCommand, tier: tier, java21: slot.java21}
		p, _, err := h.configuration()
		if err != nil {
			return err
		}
		if err := h.verifyUbuntuGuest(ctx, p); err != nil {
			return err
		}
		return recoverConfiguredHost(ctx, h, name, expectedID)
	}
	return recoverConfiguredSlot(ctx, runCommand, name, expectedID, tier)
}

func recoverConfiguredSlot(ctx context.Context, run command, name, expectedID string, tier experiment.ReferenceTier) error {
	return recoverConfiguredHost(ctx, &Host{run: run, tier: tier}, name, expectedID)
}

// RecoverUbuntuM0 proves stop only through the pinned Ubuntu Engine. The same
// ownership and external dispatch fence requirements as RecoverM0Slot apply.
func RecoverUbuntuM0(ctx context.Context, expectedID string) error {
	h, err := ubuntuHost(ctx)
	if err != nil {
		return err
	}
	return recoverConfiguredHost(ctx, h, container, expectedID)
}

func recoverConfiguredHost(ctx context.Context, h *Host, name, expectedID string) error {
	if !containerID.MatchString(expectedID) {
		return errors.New("original full M0 container ID required")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	h.id = expectedID
	h.active = make(map[uint64]context.CancelFunc)
	h.drained = make(chan struct{})
	p, _, err := h.configuration()
	if err != nil {
		return err
	}
	s, err := h.inspect(ctx, name)
	if err != nil {
		return errors.New("recovery sandbox inspection failed")
	}
	if s.ID != expectedID || s.Started == "" || s.Restarting || s.Paused {
		return errors.New("recovery sandbox identity or lifetime unavailable")
	}
	if err := h.validateIsolation(s, p); err != nil {
		return err
	}
	if h.tier == experiment.ReferenceAPlusBUbuntu {
		if err := h.verifySlotOwner(ctx); err != nil {
			return err
		}
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
	if err := h.validateIsolation(s, p); err != nil {
		return err
	}
	return ctx.Err()
}

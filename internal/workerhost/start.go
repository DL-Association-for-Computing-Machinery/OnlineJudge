package workerhost

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

// Identity returns the bound full container ID and process lifetime, never a
// mutable slot alias. It remains available after Stop for private audit reports.
func (h *Host) Identity() (id, started string) { return h.id, h.started }

// StartM0Slot starts only a pre-existing stopped, owned, exactly configured slot.
// The caller MUST hold its canonical clean ownership lock and persist the dirty
// marker before calling. No creation, replacement, recovery or arbitrary alias
// is supported. A non-nil Host may accompany an error: attempt Stop using an
// independent context, and never clear quarantine without confirmed termination.
func StartM0Slot(ctx context.Context, slot M0Slot) (*Host, error) {
	return startM0Slot(ctx, runCommand, slot)
}

func startM0Slot(ctx context.Context, run command, slot M0Slot) (*Host, error) {
	return startConfiguredSlot(ctx, run, slot.ContainerName(), 0)
}

func startConfiguredSlot(ctx context.Context, run command, name string, tier experiment.ReferenceTier) (*Host, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	h := &Host{run: run, tier: tier, active: make(map[uint64]context.CancelFunc), drained: make(chan struct{})}
	p, _, err := h.configuration()
	if err != nil {
		return nil, err
	}
	s, err := h.inspect(ctx, name)
	if err != nil {
		return nil, err
	}
	if !containerID.MatchString(s.ID) || s.Running || s.PID != 0 || s.Paused || s.Restarting {
		return nil, errors.New("reference slot is not safely stopped")
	}
	if err := validateIsolation(s, p); err != nil {
		return nil, err
	}
	h.id, h.started = s.ID, s.Started
	owner, err := h.docker(ctx, nil, "inspect", "--format", `{{index .Config.Labels "owner"}}`, h.id)
	if err != nil || strings.TrimSpace(string(owner)) != "onlinejudge.desktop-test" {
		return nil, errors.New("reference slot owner mismatch")
	}
	_, startErr := h.docker(ctx, nil, "start", h.id)
	// Even a lost start acknowledgement needs a bound lifetime for Stop. This
	// read has its own budget so cancellation cannot turn unknown start into clean.
	observe, done := context.WithTimeout(context.Background(), 5*time.Second)
	after, observeErr := h.inspect(observe, h.id)
	done()
	if observeErr != nil || after.ID != h.id || !after.Running || after.PID <= 1 || after.Started == "" || after.Started == s.Started || after.Paused || after.Restarting {
		return h, errors.Join(errors.New("reference slot start unconfirmed"), startErr, observeErr)
	}
	h.started = after.Started
	if startErr != nil {
		return h, startErr
	}
	if err := h.Verify(ctx, p); err != nil {
		return h, err
	}
	return h, nil
}

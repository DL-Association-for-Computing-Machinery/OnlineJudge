package workerhost

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestReferenceSlotAndHostRuntimeBinding(t *testing.T) {
	slot := ReferenceSlot{}
	if slot.ContainerName() == (M0Slot{}).ContainerName() || slot.LockName() == (M0Slot{}).LockName() {
		t.Fatal("reference aliases M0")
	}
	if _, err := ParseM0Slot("reference"); err == nil {
		t.Fatal("M0 parser admitted reference")
	}
	f := &fakeHost{s: goodState()}
	h, err := openConfiguredSlot(context.Background(), f.run, slot.ContainerName(), experiment.ReferenceSmall)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Verify(context.Background(), experiment.PinnedEnvironment()); err == nil {
		t.Fatal("reference host accepted M0 profile")
	}
	if err := h.Verify(context.Background(), experiment.PinnedReferenceEnvironment()); err != nil {
		t.Fatal(err)
	}
	m0, err := open(context.Background(), f.run)
	if err != nil {
		t.Fatal(err)
	}
	if err := m0.Verify(context.Background(), experiment.PinnedReferenceEnvironment()); err == nil {
		t.Fatal("M0 host widened")
	}
}

func TestReferenceSlotUnknownStartReceiptStillStops(t *testing.T) {
	f := &fakeHost{s: goodState()}
	f.s.Running = false
	f.s.PID = 0
	initial := true
	run := func(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
		if len(args) >= 6 && args[2] == "inspect" && args[4] == inspection && initial {
			initial = false
			if args[5] != (ReferenceSlot{}).ContainerName() {
				t.Fatal("wrong initial alias")
			}
		}
		if len(args) >= 6 && args[2] == "inspect" && strings.Contains(args[4], ".Config.Labels") {
			return []byte("onlinejudge.desktop-test"), nil
		}
		if len(args) >= 4 && args[2] == "start" {
			if args[3] != f.s.ID {
				t.Fatal("mutable start alias")
			}
			f.s.Running = true
			f.s.PID = 42
			f.s.Started = "reference-start"
			return nil, errors.New("unknown receipt")
		}
		return f.run(ctx, input, name, args...)
	}
	h, err := startConfiguredSlot(context.Background(), run, (ReferenceSlot{}).ContainerName(), experiment.ReferenceSmall)
	if err == nil || h == nil || h.tier != experiment.ReferenceSmall {
		t.Fatal("lost start binding", err)
	}
	if err := h.Stop(context.Background()); err != nil {
		t.Fatal("unknown receipt termination", err)
	}
	if f.s.Running || f.s.PID != 0 {
		t.Fatal("sandbox still active")
	}
}

func TestReferenceRecoveryOnlyConfirmsOriginalFixedSlot(t *testing.T) {
	for _, scenario := range []string{"stopped", "running", "kill", "changed-id", "isolation"} {
		t.Run(scenario, func(t *testing.T) {
			f := &fakeHost{s: goodState()}
			id := f.s.ID
			if scenario == "stopped" {
				f.s.Running = false
				f.s.PID = 0
			}
			if scenario == "kill" {
				f.fail = "kill"
			}
			if scenario == "changed-id" {
				f.s.ID = strings.Repeat("b", 64)
			}
			if scenario == "isolation" {
				f.s.Network = "host"
			}
			err := recoverConfiguredSlot(context.Background(), f.run, (ReferenceSlot{}).ContainerName(), id, experiment.ReferenceSmall)
			if (err == nil) != (scenario == "stopped" || scenario == "running") {
				t.Fatal("recovery outcome", err)
			}
			for _, args := range f.calls {
				if len(args) > 3 && (args[3] == "start" || args[3] == "exec") {
					t.Fatal("recovery started or probed executor")
				}
			}
		})
	}
}

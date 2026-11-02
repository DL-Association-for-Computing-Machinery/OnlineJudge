package workerhost

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestRecoveryTermination(t *testing.T) {
	for _, scenario := range []string{"running", "stopped", "top", "kill", "pid"} {
		t.Run(scenario, func(t *testing.T) {
			f := &fakeHost{s: goodState(), fail: scenario}
			if scenario == "stopped" {
				f.s.Running, f.s.PID = false, 0
			}
			err := recoverSlot(context.Background(), f.run, M0Slot{}, f.s.ID)
			if (err == nil) != (scenario == "running" || scenario == "stopped") {
				t.Fatalf("recovery: %v", err)
			}
			kills, probes := 0, 0
			for _, c := range f.calls {
				switch c[3] {
				case "kill":
					kills++
				case "run":
					probes++
				case "exec", "start", "restart", "rm":
					t.Fatal("recovery performed non-termination operation")
				}
			}
			if scenario == "stopped" && (kills != 0 || probes != 0 || len(f.calls) != 2) {
				t.Fatal("already stopped recovery did not use two inspections only")
			}
			if scenario == "running" && (kills != 1 || probes != 2 || f.s.Running || f.s.PID != 0) {
				t.Fatal("running recovery omitted stop/PID evidence")
			}
		})
	}
}

func TestRecoveryRejectsUncertainState(t *testing.T) {
	for name, change := range map[string]func(*state){
		"id":             func(s *state) { s.ID = strings.Repeat("b", 64) },
		"lifetime":       func(s *state) { s.Started = "" },
		"image":          func(s *state) { s.Image = "other" },
		"mounts":         func(s *state) { s.Mounts = 1 },
		"network":        func(s *state) { s.Network = "host" },
		"restart policy": func(s *state) { s.Restart = "always" },
		"restarting":     func(s *state) { s.Restarting = true },
		"paused":         func(s *state) { s.Paused = true },
		"stopped PID":    func(s *state) { s.Running = false },
		"running PID":    func(s *state) { s.PID = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeHost{s: goodState()}
			id := f.s.ID
			change(&f.s)
			if err := recoverSlot(context.Background(), f.run, M0Slot{}, id); err == nil {
				t.Fatal("uncertain state recovered")
			}
			for _, c := range f.calls {
				if c[3] != "inspect" {
					t.Fatal("mutated incompatible container")
				}
			}
		})
	}
}

func TestRecoveryReadbackAndBudgets(t *testing.T) {
	for _, scenario := range []string{"inspect", "replacement", "restart", "isolation", "running", "paused", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := &fakeHost{s: goodState()}
			f.s.Running, f.s.PID = false, 0
			id := f.s.ID
			calls := 0
			run := func(ctx context.Context, in io.Reader, cmd string, args ...string) ([]byte, error) {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 20*time.Second {
					t.Fatal("recovery has no total budget")
				}
				calls++
				if calls == 1 && args[len(args)-1] != container {
					t.Fatal("first lookup did not use fixed slot")
				}
				if calls == 2 {
					if args[len(args)-1] != id {
						t.Fatal("readback did not bind immutable ID")
					}
					switch scenario {
					case "inspect":
						return nil, errors.New("private inspection error")
					case "replacement":
						f.s.ID = strings.Repeat("b", 64)
					case "restart":
						f.s.Started = "other lifetime"
					case "isolation":
						f.s.Network = "host"
					case "running":
						f.s.Running = true
					case "paused":
						f.s.Paused = true
					case "canceled":
						cancel()
					}
				}
				return f.run(ctx, in, cmd, args...)
			}
			if err := recoverSlot(ctx, run, M0Slot{}, id); err == nil || strings.Contains(err.Error(), "private") {
				t.Fatalf("unsafe readback result: %v", err)
			}
		})
	}
	f := &fakeHost{s: goodState()}
	if err := recoverSlot(context.Background(), f.run, M0Slot{}, "short"); err == nil || len(f.calls) != 0 {
		t.Fatal("invalid ID reached Docker")
	}
}

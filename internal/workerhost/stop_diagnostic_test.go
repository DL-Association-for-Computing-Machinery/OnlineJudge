package workerhost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestStopFailureStagesAndEvidence(t *testing.T) {
	for _, test := range []struct {
		name, stage, code                  string
		observed, confirmed, probe, probes int
	}{
		{"inspect-before", "inspect_before", "unknown", 0, 0, 0, 0},
		{"lifetime", "inspect_before", "lifetime_changed", 0, 0, 0, 0},
		{"enumerate", "enumerate", "unknown", 0, 0, 0, 0},
		{"parse", "enumerate", "invalid_evidence", 0, 0, 0, 0},
		{"kill", "kill", "unknown", 2, 0, 0, 0},
		{"enumerate-and-kill", "kill", "unknown", 0, 0, 0, 0},
		{"inspect-after", "inspect_after", "unknown", 2, 0, 0, 0},
		{"state-after", "inspect_after", "state_unconfirmed", 2, 0, 0, 0},
		{"pid", "pid_probe", "pid_present", 2, 0, 1, 1},
		{"proc", "pid_probe", "proc_unavailable", 2, 0, 1, 1},
		{"docker", "pid_probe", "command_exit", 2, 0, 1, 1},
		{"second", "pid_probe", "pid_present", 2, 1, 2, 2},
		{"deadline", "pid_probe", "deadline", 2, 0, 1, 1},
		{"complete", "complete", "canceled", 2, 2, 0, 2},
		{"complete-deadline", "complete", "deadline", 2, 2, 0, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			h, f := opened(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.name == "complete-deadline" {
				var done context.CancelFunc
				ctx, done = context.WithTimeout(ctx, time.Second)
				defer done()
			}
			inspects, probes, kills := 0, 0, 0
			secret := errors.New("synthetic-secret /private/path pid=123456")
			h.run = func(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 10*time.Second {
					t.Fatal("changed total stop budget")
				}
				switch args[2] {
				case "inspect":
					inspects++
					if test.name == "inspect-before" && inspects == 1 || test.name == "inspect-after" && inspects == 2 {
						return nil, secret
					}
					if test.name == "lifetime" {
						f.s.Started = "changed"
					}
					if test.name == "state-after" && inspects == 2 {
						f.s.Running = true
					}
				case "top":
					if test.name == "enumerate" || test.name == "enumerate-and-kill" {
						return nil, secret
					}
					if test.name == "parse" {
						return []byte("PID\n123456\n"), nil
					}
				case "kill":
					kills++
					if test.name == "kill" || test.name == "enumerate-and-kill" {
						return nil, secret
					}
				case "run":
					probes++
					switch test.name {
					case "pid":
						return nil, &commandFailure{code: "command_exit", exit: probePIDPresent}
					case "proc":
						return nil, &commandFailure{code: "command_exit", exit: probeProcUnavailable}
					case "docker":
						return nil, &commandFailure{code: "command_exit", exit: 125}
					case "second":
						if probes == 2 {
							return nil, &commandFailure{code: "command_exit", exit: probePIDPresent}
						}
					case "deadline":
						return nil, errors.Join(secret, context.DeadlineExceeded)
					case "complete":
						if probes == 2 {
							cancel()
							return nil, nil
						}
					case "complete-deadline":
						if probes == 2 {
							<-ctx.Done()
							return nil, nil
						}
					}
				}
				return f.run(ctx, input, name, args...)
			}
			err := h.Stop(ctx)
			d := StopDetails(err)
			if err == nil || d.Stage != test.stage || d.Code != test.code || d.Observed != test.observed || d.Confirmed != test.confirmed || d.Probe != test.probe || probes != test.probes {
				t.Fatalf("diagnostic=%+v probes=%d", d, probes)
			}
			if test.name != "inspect-before" && test.name != "lifetime" && kills != 1 {
				t.Fatal("missing kill attempt")
			}
			wantSteps := []string{"drain", "inspect_before", "enumerate", "kill", "inspect_after", "pid_probe"}
			switch test.stage {
			case "inspect_before":
				wantSteps = wantSteps[:2]
			case "enumerate", "kill":
				wantSteps = wantSteps[:4]
			case "inspect_after":
				wantSteps = wantSteps[:5]
			}
			if len(d.Steps) != len(wantSteps) {
				t.Fatalf("unreached or missing steps: %+v", d.Steps)
			}
			for i, step := range d.Steps {
				if step.Stage != wantSteps[i] || step.ElapsedMS < 0 || step.ElapsedMS > 60000 {
					t.Fatal("stop step order or bounds", d.Steps)
				}
				if step.Stage == "kill" && (!step.CallAttempted || step.ContextBefore != "active" || step.LocalProcessStart != "unknown") {
					t.Fatal("test double acquired local process evidence", step)
				}
				if step.Stage == "pid_probe" && step.Count != probes {
					t.Fatal("PID aggregate count changed", step)
				}
			}
			if (test.name == "deadline" || test.name == "complete-deadline") && !errors.Is(err, context.DeadlineExceeded) || test.name == "complete" && !errors.Is(err, context.Canceled) {
				t.Fatal("lost context identity")
			}
			raw, e := json.Marshal(d)
			if e != nil || strings.Contains(err.Error()+string(raw), "synthetic-secret") || strings.Contains(string(raw), "123456") {
				t.Fatal("stop leaked private evidence")
			}
			if err := h.Stop(context.Background()); StopDetails(err).Code != "already_fenced" {
				t.Fatal("stop reused fenced host")
			}
		})
	}
	if StopDetails(nil) != nil {
		t.Fatal("successful stop acquired a failure")
	}
}

func TestStopDrainDeadline(t *testing.T) {
	h, _ := opened(t)
	_, finish, err := h.begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	err = h.Stop(ctx)
	if d := StopDetails(err); d.Stage != "drain" || d.Code != "deadline" || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("drain=%+v", d)
	}
}

func TestPIDProbeExitMeaning(t *testing.T) {
	// Run only the trusted probe script in the toolchain process namespace.
	// PID 1 exists; a non-numeric nonexistent proc entry does not.
	for _, test := range []struct {
		pid  string
		exit int
	}{{"1", probePIDPresent}, {"nonexistent-oj-probe", 0}} {
		args := PIDProbeArgs(test.pid)
		_, err := runCommand(context.Background(), nil, "/bin/sh", args[len(args)-4:]...)
		if test.exit == 0 {
			if err != nil {
				t.Fatal(err)
			}
		} else {
			var failure *commandFailure
			if !errors.As(err, &failure) || failure.exit != test.exit {
				t.Fatalf("probe result: %v", err)
			}
		}
	}
	args := PIDProbeArgs("1")
	args = append([]string(nil), args[len(args)-4:]...)
	args[1] = "test() { return 1; }; " + args[1]
	_, err := runCommand(context.Background(), nil, "/bin/sh", args...)
	var failure *commandFailure
	if !errors.As(err, &failure) || failure.exit != probeProcUnavailable {
		t.Fatalf("missing proc evidence: %v", err)
	}
	// An expression/command error is not evidence of an absent target. Only the
	// first procfs test succeeds; the target test returns the abnormal status 2.
	args = PIDProbeArgs("1")
	args = append([]string(nil), args[len(args)-4:]...)
	args[1] = "calls=0; test() { calls=$((calls+1)); if [ \"$calls\" -eq 1 ]; then return 0; else return 2; fi; }; " + args[1]
	_, err = runCommand(context.Background(), nil, "/bin/sh", args...)
	if !errors.As(err, &failure) || failure.exit == 0 || failure.exit == probePIDPresent || failure.exit == probeProcUnavailable {
		t.Fatalf("abnormal target test accepted: %v", err)
	}
}

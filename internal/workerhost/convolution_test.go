package workerhost

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func convolutionState(t *testing.T) state {
	t.Helper()
	c, err := experiment.ResolveReferenceTier(experiment.ReferenceConvolution)
	if err != nil {
		t.Fatal(err)
	}
	s := goodState()
	s.Entrypoint, s.Cmd = c.Runtime.SandboxArgs, c.Runtime.SandboxCmd
	return s
}

func TestConvolutionSlotIsolationAndHostBudget(t *testing.T) {
	ctx := context.Background()
	slot, err := ReferenceSlotForTier(experiment.ReferenceConvolution)
	if err != nil || slot.ContainerName() != "oj-reference-convolution-sandbox" || slot.LockName() != "slot-reference-convolution.lock" {
		t.Fatal("convolution fixed slot", err)
	}
	for _, tier := range []experiment.ReferenceTier{experiment.ReferenceSmall, experiment.ReferenceMedium, experiment.ReferenceMediumBulk, experiment.ReferencePrimality, experiment.ReferenceStaticRMQ, experiment.ReferenceStaticRangeSum, experiment.ReferenceAssociativeArray} {
		old, err := ReferenceSlotForTier(tier)
		if err != nil || old.ContainerName() == slot.ContainerName() || old.LockName() == slot.LockName() {
			t.Fatal("new slot aliases an old reference slot", tier, err)
		}
	}
	for _, name := range []string{"primary", "secondary"} {
		old, err := ParseM0Slot(name)
		if err != nil || old.ContainerName() == slot.ContainerName() || old.LockName() == slot.LockName() {
			t.Fatal("new slot aliases M0", name, err)
		}
	}
	if _, err := ParseM0Slot("convolution"); err == nil {
		t.Fatal("M0 parser admitted convolution")
	}
	f := &fakeHost{s: convolutionState(t)}
	h, err := openConfiguredSlot(ctx, f.run, slot.ContainerName(), experiment.ReferenceConvolution)
	if err != nil {
		t.Fatal(err)
	}
	runtime, files, err := h.configuration()
	want, resolveErr := experiment.ResolveReferenceTier(experiment.ReferenceConvolution)
	if err != nil || resolveErr != nil || h.tier != experiment.ReferenceConvolution || !reflect.DeepEqual(runtime, want.Runtime) || files != want.Files || files.PayloadBytes != 2<<30 {
		t.Fatal("host normalized convolution tier or ledger", err, resolveErr)
	}
	if err := h.Verify(ctx, want.Runtime); err != nil {
		t.Fatal("valid convolution runtime rejected", err)
	}
	for _, tier := range []experiment.ReferenceTier{experiment.ReferenceSmall, experiment.ReferenceMedium, experiment.ReferenceMediumBulk, experiment.ReferencePrimality, experiment.ReferenceStaticRMQ, experiment.ReferenceStaticRangeSum, experiment.ReferenceAssociativeArray} {
		old, _ := experiment.ResolveReferenceTier(tier)
		if err := h.Verify(ctx, old.Runtime); err == nil {
			t.Fatal("new host accepted old runtime", tier)
		}
		oldSlot, err := ReferenceSlotForTier(tier)
		if err != nil {
			t.Fatal(err)
		}
		oldState := mediumState(t)
		if tier == experiment.ReferenceSmall {
			oldState = goodState()
		}
		oldFake := &fakeHost{s: oldState}
		oldHost, err := openConfiguredSlot(ctx, oldFake.run, oldSlot.ContainerName(), tier)
		if err != nil {
			t.Fatal(err)
		}
		if err := oldHost.Verify(ctx, want.Runtime); err == nil {
			t.Fatal("old host borrowed convolution runtime", tier)
		}
	}
	transfers := 0
	h.stream = func(_ context.Context, _ io.Reader, out io.Writer, _ int64, _ string, _ ...string) error {
		transfers++
		_, err := io.WriteString(out, "x\n200")
		return err
	}
	h.filePayload = 1 << 30
	if raw, err := h.ReadFile(ctx, "owned", 1); err != nil || string(raw) != "x" || transfers != 1 {
		t.Fatal("host retained old 1GiB cap", err)
	}
	h.filePayload = (2 << 30) - 1
	if _, err := h.ReadFile(ctx, "owned", 1); err != nil || h.filePayload != 2<<30 || transfers != 2 {
		t.Fatal("exact 2GiB host cap refused", err)
	}
	if _, err := h.ReadFile(ctx, "owned", 1); err == nil || transfers != 2 || h.filePayload != 2<<30 || h.fileBusy.Load() || len(h.active) != 0 {
		t.Fatal("host payload plus one dispatched or retained activity")
	}
	f.response = "{}\n200"
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "http://127.0.0.1:5050/file/owned", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := h.RoundTrip(req)
	if err != nil {
		t.Fatal("exhausted file payload blocked control cleanup", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if err := h.Stop(ctx); err != nil || f.s.Running || f.s.PID != 0 {
		t.Fatal("convolution stop unconfirmed", err)
	}
	before := len(f.calls)
	if err := h.Verify(ctx, want.Runtime); err == nil || len(f.calls) != before {
		t.Fatal("convolution host reused after Stop")
	}
}

func TestConvolutionStartRejectsOldCmdAndKeepsOriginalIdentity(t *testing.T) {
	for _, mode := range []string{"good", "output8MiB", "owner", "running", "lost-ack"} {
		t.Run(mode, func(t *testing.T) {
			slot, _ := ReferenceSlotForTier(experiment.ReferenceConvolution)
			f := &fakeHost{s: convolutionState(t)}
			f.s.Running, f.s.PID = false, 0
			if mode == "running" {
				f.s.Running, f.s.PID = true, 42
			}
			if mode == "output8MiB" {
				f.s.Cmd = mediumState(t).Cmd
			}
			original, starts := f.s.ID, 0
			run := func(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
				if len(args) >= 6 && args[2] == "inspect" && strings.Contains(args[4], ".Config.Labels") {
					if mode == "owner" {
						return []byte("other"), nil
					}
					return []byte("onlinejudge.desktop-test"), nil
				}
				if len(args) >= 4 && args[2] == "start" {
					starts++
					if args[3] != original {
						t.Fatal("start used mutable alias")
					}
					f.s.Running, f.s.PID, f.s.Started = true, 42, "convolution-start"
					if mode == "lost-ack" {
						return nil, errors.New("unknown start receipt")
					}
					return []byte(original), nil
				}
				return f.run(ctx, input, name, args...)
			}
			h, err := startConfiguredSlot(context.Background(), run, slot.ContainerName(), experiment.ReferenceConvolution)
			if mode == "good" || mode == "lost-ack" {
				if h == nil || starts != 1 || (err == nil) != (mode == "good") {
					t.Fatal("start/lost receipt binding", err)
				}
				id, started := h.Identity()
				if id != original || started != "convolution-start" {
					t.Fatal("start did not bind original full ID/lifetime")
				}
				if err := h.Stop(context.Background()); err != nil {
					t.Fatal("original lifetime stop", err)
				}
			} else if err == nil || starts != 0 {
				t.Fatal("incompatible slot started", mode, err)
			}
		})
	}
}

func TestConvolutionRecoveryAndStopFailureRemainFailClosed(t *testing.T) {
	for _, mode := range []string{"stopped", "running", "wrong-id", "output8MiB", "kill-failure"} {
		t.Run(mode, func(t *testing.T) {
			slot, _ := ReferenceSlotForTier(experiment.ReferenceConvolution)
			f := &fakeHost{s: convolutionState(t)}
			id := f.s.ID
			if mode == "stopped" {
				f.s.Running, f.s.PID = false, 0
			}
			if mode == "wrong-id" {
				f.s.ID = strings.Repeat("b", 64)
			}
			if mode == "output8MiB" {
				f.s.Cmd = mediumState(t).Cmd
			}
			if mode == "kill-failure" {
				f.fail = "kill"
			}
			err := recoverConfiguredSlot(context.Background(), f.run, slot.ContainerName(), id, experiment.ReferenceConvolution)
			if (err == nil) != (mode == "stopped" || mode == "running") {
				t.Fatal("recovery confirmation gate", mode, err)
			}
			for _, call := range f.calls {
				if len(call) > 3 && (call[3] == "start" || call[3] == "exec") {
					t.Fatal("recovery started or probed executor")
				}
			}
		})
	}
}

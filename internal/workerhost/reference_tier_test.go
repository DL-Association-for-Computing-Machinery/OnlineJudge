package workerhost

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func mediumState(t *testing.T) state {
	t.Helper()
	c, err := experiment.ResolveReferenceTier(experiment.ReferenceMedium)
	if err != nil {
		t.Fatal(err)
	}
	s := goodState()
	s.Entrypoint, s.Cmd = c.Runtime.SandboxArgs, c.Runtime.SandboxCmd
	return s
}

func TestReferenceTierRejectsUnknownBeforeDispatch(t *testing.T) {
	ctx := context.Background()
	for _, tier := range []experiment.ReferenceTier{0, 4, 255} {
		slot, err := ReferenceSlotForTier(tier)
		if err == nil || slot.ContainerName() != "" || slot.LockName() != "" {
			t.Fatal("unknown tier became a usable slot")
		}
		if h, err := OpenReferenceSlot(ctx, slot); err == nil || h != nil {
			t.Fatal("invalid slot opened")
		}
		if h, err := StartReferenceSlot(ctx, slot); err == nil || h != nil {
			t.Fatal("invalid slot started")
		}
		if err := RecoverReferenceSlot(ctx, slot, strings.Repeat("a", 64)); err == nil {
			t.Fatal("invalid slot recovered")
		}
	}
	f := &fakeHost{s: mediumState(t)}
	if _, err := openConfiguredSlot(ctx, f.run, "oj-reference-medium-sandbox", 255); err == nil {
		t.Fatal("unknown host tier opened")
	}
	if _, err := startConfiguredSlot(ctx, f.run, "oj-reference-medium-sandbox", 255); err == nil {
		t.Fatal("unknown host tier started")
	}
	if err := recoverConfiguredSlot(ctx, f.run, "oj-reference-medium-sandbox", f.s.ID, 255); err == nil {
		t.Fatal("unknown host tier recovered")
	}
	h := &Host{run: f.run, tier: 255, id: f.s.ID, started: f.s.Started, active: map[uint64]context.CancelFunc{}, drained: make(chan struct{})}
	if err := h.Verify(ctx, experiment.PinnedEnvironment()); err == nil {
		t.Fatal("unknown Verify fell back to M0")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:5050/run", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.RoundTrip(req); err == nil {
		t.Fatal("unknown control tier dispatched")
	}
	if _, err := h.UploadFile(ctx, nil); err == nil {
		t.Fatal("unknown upload tier dispatched")
	}
	if _, err := h.ReadFile(ctx, "owned", 0); err == nil {
		t.Fatal("unknown download tier dispatched")
	}
	if err := h.fileExchange(ctx, nil, &bytes.Buffer{}, 0, nil); err == nil {
		t.Fatal("unknown raw exchange dispatched")
	}
	if len(f.calls) != 0 || len(h.active) != 0 || h.fileBusy.Load() {
		t.Fatal("invalid configuration performed control IO or retained activity")
	}
}

func TestMediumIsolationCmdAndLifetimeBinding(t *testing.T) {
	c, _ := experiment.ResolveReferenceTier(experiment.ReferenceMedium)
	small, _ := ReferenceSlotForTier(experiment.ReferenceSmall)
	medium, _ := ReferenceSlotForTier(experiment.ReferenceMedium)
	if small.ContainerName() != (ReferenceSlot{}).ContainerName() || small.LockName() != (ReferenceSlot{}).LockName() || medium.ContainerName() == small.ContainerName() || medium.LockName() == small.LockName() {
		t.Fatal("slot aliases")
	}
	for _, scenario := range []string{"good", "empty-cmd", "changed-cmd", "legacy-entrypoint"} {
		t.Run(scenario, func(t *testing.T) {
			s := mediumState(t)
			switch scenario {
			case "empty-cmd":
				s.Cmd = nil
			case "changed-cmd":
				s.Cmd[0] = "-other"
			case "legacy-entrypoint":
				s.Entrypoint = experiment.PinnedEnvironment().SandboxArgs
			}
			f := &fakeHost{s: s}
			h, err := openConfiguredSlot(context.Background(), f.run, medium.ContainerName(), experiment.ReferenceMedium)
			if (err == nil) != (scenario == "good") {
				t.Fatal("medium isolation", scenario, err)
			}
			if h != nil {
				if err := h.Verify(context.Background(), experiment.PinnedReferenceEnvironment()); err == nil {
					t.Fatal("medium host accepted small")
				}
				if err := h.Verify(context.Background(), c.Runtime); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	for _, cmd := range [][]string{nil, {}} {
		s := goodState()
		s.Cmd = cmd
		if err := validateIsolation(s, experiment.PinnedEnvironment()); err != nil {
			t.Fatal("old nil/empty Cmd compatibility changed", err)
		}
	}
}

func TestMediumHostPayloadLedgerAndDrift(t *testing.T) {
	f := &fakeHost{s: mediumState(t)}
	h, err := openConfiguredSlot(context.Background(), f.run, "oj-reference-medium-sandbox", experiment.ReferenceMedium)
	if err != nil {
		t.Fatal(err)
	}
	transfers := 0
	h.stream = func(_ context.Context, _ io.Reader, out io.Writer, _ int64, _ string, _ ...string) error {
		transfers++
		_, err := io.WriteString(out, "x\n200")
		return err
	}
	h.filePayload = 128 << 20
	if raw, err := h.ReadFile(context.Background(), "owned", 2); err != nil || string(raw) != "x" || transfers != 1 {
		t.Fatal("medium host retained old cumulative cap", err)
	}
	h.filePayload = 256 << 20
	if _, err := h.ReadFile(context.Background(), "owned", 1); err == nil || transfers != 1 {
		t.Fatal("medium cumulative cap bypassed")
	}
	h.filePayload = 0
	f.s.Cmd[0] = "-drift"
	if _, err := h.ReadFile(context.Background(), "owned", 1); err == nil || transfers != 1 {
		t.Fatal("raw file dispatch ignored Cmd drift")
	}
	f.response = "{}\n200"
	req, err := http.NewRequest("POST", "http://127.0.0.1:5050/run", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.RoundTrip(req); err == nil {
		t.Fatal("control dispatch ignored Cmd drift")
	}
}

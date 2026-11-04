package workerhost

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestAssociativeArrayHostKeepsTierAndExactPayloadCeiling(t *testing.T) {
	slot, err := ReferenceSlotForTier(experiment.ReferenceAssociativeArray)
	medium, _ := ReferenceSlotForTier(experiment.ReferenceMedium)
	if err != nil || slot.ContainerName() != medium.ContainerName() || slot.LockName() != medium.LockName() || slot.tier != experiment.ReferenceAssociativeArray {
		t.Fatal("source slot lost fixed identity or canonical lock")
	}
	f := &fakeHost{s: mediumState(t)}
	h, err := openConfiguredSlot(context.Background(), f.run, slot.ContainerName(), experiment.ReferenceAssociativeArray)
	if err != nil {
		t.Fatal(err)
	}
	runtime, files, err := h.configuration()
	want, _ := experiment.ResolveReferenceTier(experiment.ReferenceAssociativeArray)
	if err != nil || !reflect.DeepEqual(runtime, want.Runtime) || files != want.Files || files.PayloadBytes != 1<<30 {
		t.Fatal("host normalized tier or disagreed with cache ledger")
	}
	for _, tier := range []experiment.ReferenceTier{experiment.ReferenceMedium, experiment.ReferencePrimality, experiment.ReferenceStaticRangeSum} {
		other, _ := experiment.ResolveReferenceTier(tier)
		if err := h.Verify(context.Background(), other.Runtime); err == nil {
			t.Fatal("host accepted another runtime")
		}
	}
	transfers := 0
	h.stream = func(_ context.Context, _ io.Reader, out io.Writer, _ int64, _ string, _ ...string) error {
		transfers++
		_, err := io.WriteString(out, "x\n200")
		return err
	}
	h.filePayload = 512 << 20
	if raw, err := h.ReadFile(context.Background(), "owned", 1); err != nil || string(raw) != "x" || transfers != 1 {
		t.Fatal("source retained previous host payload ceiling", err)
	}
	h.filePayload = (1 << 30) - 1
	if _, err := h.ReadFile(context.Background(), "owned", 1); err != nil || h.filePayload != 1<<30 || transfers != 2 {
		t.Fatal("exact host payload ceiling rejected", err)
	}
	if _, err := h.ReadFile(context.Background(), "owned", 1); err == nil || transfers != 2 || h.fileBusy.Load() || len(h.active) != 0 {
		t.Fatal("host overflow dispatched or retained admission")
	}
}

package workerhost

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestStaticRangeSumHostKeepsTierAndPayloadLedger(t *testing.T) {
	slot, err := ReferenceSlotForTier(experiment.ReferenceStaticRangeSum)
	medium, _ := ReferenceSlotForTier(experiment.ReferenceMedium)
	if err != nil || slot.ContainerName() != medium.ContainerName() || slot.LockName() != medium.LockName() || slot.tier != experiment.ReferenceStaticRangeSum {
		t.Fatal("source slot lost its identity or canonical lock")
	}
	f := &fakeHost{s: mediumState(t)}
	h, err := openConfiguredSlot(context.Background(), f.run, slot.ContainerName(), experiment.ReferenceStaticRangeSum)
	if err != nil {
		t.Fatal(err)
	}
	runtime, files, err := h.configuration()
	want, _ := experiment.ResolveReferenceTier(experiment.ReferenceStaticRangeSum)
	if err != nil || h.tier != experiment.ReferenceStaticRangeSum || !reflect.DeepEqual(runtime, want.Runtime) || files != want.Files || files.PayloadBytes != 512<<20 {
		t.Fatal("host normalized tier or disagreed with cache accounting")
	}
	for _, tier := range []experiment.ReferenceTier{experiment.ReferenceMedium, experiment.ReferencePrimality, experiment.ReferenceStaticRMQ} {
		other, _ := experiment.ResolveReferenceTier(tier)
		if err := h.Verify(context.Background(), other.Runtime); err == nil {
			t.Fatal("host accepted another source runtime")
		}
	}
	transfers := 0
	h.stream = func(_ context.Context, _ io.Reader, out io.Writer, _ int64, _ string, _ ...string) error {
		transfers++
		_, err := io.WriteString(out, "x\n200")
		return err
	}
	h.filePayload = 256 << 20
	if raw, err := h.ReadFile(context.Background(), "owned", 1); err != nil || string(raw) != "x" || transfers != 1 {
		t.Fatal("source host retained the old cumulative ceiling", err)
	}
	h.filePayload = (512 << 20) - 1
	if _, err := h.ReadFile(context.Background(), "owned", 1); err != nil || h.filePayload != 512<<20 || transfers != 2 {
		t.Fatal("exact host payload ceiling rejected", err)
	}
	if _, err := h.ReadFile(context.Background(), "owned", 1); err == nil || transfers != 2 || h.fileBusy.Load() || len(h.active) != 0 {
		t.Fatal("host overflow dispatched or retained activity")
	}
}

package workerhost

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestStaticRMQHostKeepsTierAndItsCumulativePayload(t *testing.T) {
	slot, err := ReferenceSlotForTier(experiment.ReferenceStaticRMQ)
	m, _ := ReferenceSlotForTier(experiment.ReferenceMedium)
	if err != nil || slot.ContainerName() != m.ContainerName() || slot.LockName() != m.LockName() || slot.tier != experiment.ReferenceStaticRMQ {
		t.Fatal("staticrmq slot lost its identity or canonical lock")
	}
	f := &fakeHost{s: mediumState(t)}
	h, err := openConfiguredSlot(context.Background(), f.run, slot.ContainerName(), experiment.ReferenceStaticRMQ)
	if err != nil {
		t.Fatal(err)
	}
	runtime, files, err := h.configuration()
	want, _ := experiment.ResolveReferenceTier(experiment.ReferenceStaticRMQ)
	if err != nil || h.tier != experiment.ReferenceStaticRMQ || !reflect.DeepEqual(runtime, want.Runtime) || files != want.Files || files.PayloadBytes != 512<<20 {
		t.Fatal("host normalized tier or disagreed with cache budget")
	}
	for _, tier := range []experiment.ReferenceTier{experiment.ReferenceSmall, experiment.ReferenceMedium, experiment.ReferenceMediumBulk, experiment.ReferencePrimality, experiment.ReferenceStaticRangeSum, experiment.ReferenceAssociativeArray} {
		old, _ := experiment.ResolveReferenceTier(tier)
		if err := h.Verify(context.Background(), old.Runtime); err == nil {
			t.Fatal("host accepted another runtime")
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
		t.Fatal("host retained the old cumulative ceiling", err)
	}
	h.filePayload = (512 << 20) - 1
	if _, err := h.ReadFile(context.Background(), "owned", 1); err != nil || h.filePayload != 512<<20 || transfers != 2 {
		t.Fatal("exact host ceiling rejected", err)
	}
	if _, err := h.ReadFile(context.Background(), "owned", 1); err == nil || transfers != 2 || h.fileBusy.Load() || len(h.active) != 0 {
		t.Fatal("host overflow dispatched or retained activity")
	}
}

func TestMediumAndBulkHostsAcceptTheirSharedRuntime(t *testing.T) {
	for _, pair := range [][2]experiment.ReferenceTier{{experiment.ReferenceMedium, experiment.ReferenceMediumBulk}, {experiment.ReferenceMediumBulk, experiment.ReferenceMedium}} {
		slot, err := ReferenceSlotForTier(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		f := &fakeHost{s: mediumState(t)}
		h, err := openConfiguredSlot(context.Background(), f.run, slot.ContainerName(), pair[0])
		if err != nil {
			t.Fatal(err)
		}
		other, err := experiment.ResolveReferenceTier(pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if err := h.Verify(context.Background(), other.Runtime); err != nil {
			t.Fatal("shared medium runtime rejected", err)
		}
		static, err := experiment.ResolveReferenceTier(experiment.ReferenceStaticRMQ)
		if err != nil {
			t.Fatal(err)
		}
		if err := h.Verify(context.Background(), static.Runtime); err == nil {
			t.Fatal("medium or bulk host borrowed staticrmq runtime")
		}
	}
}

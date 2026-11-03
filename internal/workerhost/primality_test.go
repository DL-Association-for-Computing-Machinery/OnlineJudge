package workerhost

import (
	"context"
	"reflect"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestPrimalitySlotKeepsItsTierAndSharedCanonicalLock(t *testing.T) {
	p, err := ReferenceSlotForTier(experiment.ReferencePrimality)
	m, _ := ReferenceSlotForTier(experiment.ReferenceMedium)
	if err != nil || p.ContainerName() != m.ContainerName() || p.LockName() != m.LockName() || p.tier != experiment.ReferencePrimality {
		t.Fatal("dependency slot lost identity or canonical lock")
	}
	f := &fakeHost{s: mediumState(t)}
	h, err := openConfiguredSlot(context.Background(), f.run, p.ContainerName(), experiment.ReferencePrimality)
	if err != nil {
		t.Fatal(err)
	}
	runtime, files, err := h.configuration()
	c, _ := experiment.ResolveReferenceTier(experiment.ReferencePrimality)
	if err != nil || !reflect.DeepEqual(runtime, c.Runtime) || files != c.Files || files.Objects != 6 || h.tier != experiment.ReferencePrimality {
		t.Fatal("host normalized dependency tier")
	}
	old, _ := experiment.ResolveReferenceTier(experiment.ReferenceMedium)
	if err := h.Verify(context.Background(), old.Runtime); err == nil {
		t.Fatal("host accepted another runtime identity")
	}
}

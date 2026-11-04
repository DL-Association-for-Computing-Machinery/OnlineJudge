package workerhost

import (
	"context"
	"io"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestBulkReferenceSharesSlotButPreservesHostLedger(t *testing.T) {
	ctx := context.Background()
	medium, _ := ReferenceSlotForTier(experiment.ReferenceMedium)
	for _, tier := range []experiment.ReferenceTier{experiment.ReferenceMedium, experiment.ReferenceMediumBulk} {
		slot, err := ReferenceSlotForTier(tier)
		if err != nil || slot.ContainerName() != medium.ContainerName() || slot.LockName() != medium.LockName() {
			t.Fatal("bulk must contend for the same sandbox", err)
		}
		name, _, actualTier, err := slot.configuration()
		if err != nil || actualTier != tier {
			t.Fatal("slot discarded transfer tier", err)
		}
		f := &fakeHost{s: mediumState(t)}
		h, err := openConfiguredSlot(ctx, f.run, name, actualTier)
		if err != nil {
			t.Fatal(err)
		}
		transfers := 0
		h.stream = func(_ context.Context, _ io.Reader, out io.Writer, _ int64, _ string, _ ...string) error {
			transfers++
			_, err := io.WriteString(out, "x\n200")
			return err
		}
		h.filePayload = 256 << 20
		_, err = h.ReadFile(ctx, "owned", 1)
		if tier == experiment.ReferenceMedium {
			if err == nil || transfers != 0 || h.filePayload != 256<<20 {
				t.Fatal("old host budget widened")
			}
			continue
		}
		if err != nil || transfers != 1 || h.filePayload != (256<<20)+1 {
			t.Fatal("bulk host retained old ledger", err)
		}
		h.filePayload = (512 << 20) - 1
		if raw, err := h.ReadFile(ctx, "owned", 1); err != nil || string(raw) != "x" || h.filePayload != 512<<20 {
			t.Fatal("bulk host cap rejected", err)
		}
		if _, err := h.ReadFile(ctx, "owned", 1); err == nil || transfers != 2 || h.filePayload != 512<<20 {
			t.Fatal("bulk host cap bypassed")
		}
	}
}

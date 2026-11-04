package execution

import (
	"bytes"
	"context"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestMediumReferenceCompleteTransferBudgetAndClosedFactory(t *testing.T) {
	ctx := context.Background()
	f := &cacheFixture{t: t, data: map[string][]byte{}, reference: true, referenceOutput: 4 << 20}
	for _, tier := range []experiment.ReferenceTier{0, 255} {
		if g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f, tier); err == nil || g != nil {
			t.Fatal("unknown tier constructed")
		}
	}
	c, _ := experiment.ResolveReferenceTier(experiment.ReferenceMedium)
	g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f, experiment.ReferenceMedium)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Verify(ctx, c.Runtime); err != nil {
		t.Fatal(err)
	}
	o, err := g.Compile(ctx, c.Runtime, "source")
	if err != nil {
		t.Fatal(err)
	}
	f.stdout = bytes.Repeat([]byte{'x'}, 4<<20)
	for i := 0; i < 29; i++ {
		out, err := g.Run(ctx, c.Runtime, o.Artifact, bytes.Repeat([]byte{'i'}, 512<<10))
		if err != nil || !bytes.Equal(out.Stdout, f.stdout) || len(f.data) != 1 {
			t.Fatal("29-point medium budget failed", i, err)
		}
	}
	if g.totalBytes <= 128<<20 || g.totalBytes >= 256<<20 {
		t.Fatal("test did not cross old budget")
	}
	f.stdout = nil
	f.fail = "collector-OLE"
	out, err := g.Run(ctx, c.Runtime, o.Artifact, []byte("input"))
	v, e := Classify(out, false)
	if err != nil || e != nil || v != "OLE" {
		t.Fatal("medium collector overflow", err, e)
	}
	if err := g.Remove(ctx, o.Artifact); err != nil || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
		t.Fatal("medium cleanup", err)
	}
	wrong, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f, experiment.ReferenceMedium)
	if err != nil {
		t.Fatal(err)
	}
	if err := wrong.Verify(ctx, experiment.PinnedReferenceEnvironment()); err == nil {
		t.Fatal("medium accepted old runtime")
	}
	if err := NewReferenceGoJudge(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f).Verify(ctx, c.Runtime); err == nil {
		t.Fatal("old factory widened")
	}
}

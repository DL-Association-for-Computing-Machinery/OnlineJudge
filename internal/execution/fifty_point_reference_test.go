package execution

import (
	"bytes"
	"context"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestMediumFiftyPointEnvelopeAndCleanupBudget(t *testing.T) {
	ctx := context.Background()
	f := &cacheFixture{t: t, data: map[string][]byte{}, reference: true, referenceOutput: 4 << 20}
	c, err := experiment.ResolveReferenceTier(experiment.ReferenceMedium)
	if err != nil {
		t.Fatal(err)
	}
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
	for i := 0; i < 50; i++ {
		out, err := g.Run(ctx, c.Runtime, o.Artifact, bytes.Repeat([]byte{'i'}, 512<<10))
		if err != nil || !bytes.Equal(out.Stdout, f.stdout) || len(f.data) != 1 {
			t.Fatal("fifty-point envelope failed", i, err)
		}
	}
	if err := g.Remove(ctx, o.Artifact); err != nil {
		t.Fatal(err)
	}
	if f.runs != 50 || g.operations != 307 || g.totalBytes <= 236891698 || g.totalBytes >= 256<<20 || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
		t.Fatal("fifty-point budget or cleanup accounting")
	}
}

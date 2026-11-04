package execution

import (
	"bytes"
	"context"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func staticRMQAdapter(t *testing.T) (*CachedGoJudge, *cacheFixture, experiment.ExecutionProfile) {
	t.Helper()
	f := &cacheFixture{t: t, data: map[string][]byte{}, reference: true, referenceOutput: 4 << 20}
	g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f, experiment.ReferenceStaticRMQ)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Verify(context.Background(), g.runtime); err != nil {
		t.Fatal(err)
	}
	return g, f, g.runtime
}

func TestStaticRMQCompleteTwentySevenPointWorstPayload(t *testing.T) {
	ctx := context.Background()
	g, f, p := staticRMQAdapter(t)
	o, err := g.Compile(ctx, p, string(bytes.Repeat([]byte{'s'}, 1890)))
	if err != nil {
		t.Fatal(err)
	}
	f.stdout = bytes.Repeat([]byte{'x'}, 4<<20)
	remaining := 158223359
	for i := 0; i < 27; i++ {
		size := remaining / (27 - i)
		if i == 0 {
			size = 11723831
		}
		remaining -= size
		out, err := g.Run(ctx, p, o.Artifact, bytes.Repeat([]byte{'i'}, size))
		if err != nil || !bytes.Equal(out.Stdout, f.stdout) || len(f.data) != 1 || g.liveObjects != 1 {
			t.Fatal("complete source transfer or point cleanup", i, err)
		}
	}
	if remaining != 0 || g.totalBytes != 429696733 || g.totalBytes <= 256<<20 || g.totalBytes >= 512<<20 {
		t.Fatal("whole source envelope was not conservatively charged")
	}
	if err := g.Remove(ctx, o.Artifact); err != nil {
		t.Fatal(err)
	}
	if f.runs != 27 || g.operations != 169 || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
		t.Fatal("source operation envelope or final cleanup")
	}
}

func TestStaticRMQPayloadExactCapStillAllowsOwnedCleanup(t *testing.T) {
	ctx := context.Background()
	g, f, p := staticRMQAdapter(t)
	o, err := g.Compile(ctx, p, "source")
	if err != nil {
		t.Fatal(err)
	}
	g.totalBytes = (512 << 20) - 1
	uploads := f.uploads
	_, err = g.Run(ctx, p, o.Artifact, []byte{'x'})
	if err == nil || g.totalBytes != 512<<20 || f.uploads != uploads+1 || f.runs != 0 || g.liveObjects != 1 || len(f.data) != 1 {
		t.Fatal("readback exceeded exact cumulative cap or lost known cleanup", err)
	}
	if err := g.Remove(ctx, o.Artifact); err != nil || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
		t.Fatal("exhausted payload prevented owned artifact deletion", err)
	}
}

func TestStaticRMQRuntimeCannotBeBorrowedByExistingTiers(t *testing.T) {
	tiers := []experiment.ReferenceTier{experiment.ReferenceSmall, experiment.ReferenceMedium, experiment.ReferenceMediumBulk, experiment.ReferencePrimality, experiment.ReferenceStaticRMQ, experiment.ReferenceStaticRangeSum, experiment.ReferenceAssociativeArray}
	for _, tier := range tiers {
		for _, other := range tiers {
			if tier == other {
				continue
			}
			// Medium and Bulk intentionally share runtime identity; only their
			// transport ledgers differ. The positive case is checked below.
			if tier == experiment.ReferenceMedium && other == experiment.ReferenceMediumBulk || tier == experiment.ReferenceMediumBulk && other == experiment.ReferenceMedium {
				continue
			}
			f := &cacheFixture{t: t, data: map[string][]byte{}}
			g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error {
				t.Fatal("mismatched runtime reached verify")
				return nil
			}, f, tier)
			if err != nil {
				t.Fatal(err)
			}
			c, _ := experiment.ResolveReferenceTier(other)
			if err := g.Verify(context.Background(), c.Runtime); err == nil || f.compiles != 0 || f.uploads != 0 {
				t.Fatal("runtime identity widened")
			}
		}
	}
}

func TestMediumAndBulkAdaptersAcceptTheirSharedRuntime(t *testing.T) {
	for _, pair := range [][2]experiment.ReferenceTier{{experiment.ReferenceMedium, experiment.ReferenceMediumBulk}, {experiment.ReferenceMediumBulk, experiment.ReferenceMedium}} {
		f := &cacheFixture{t: t, data: map[string][]byte{}}
		verified := 0
		g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error {
			verified++
			return nil
		}, f, pair[0])
		if err != nil {
			t.Fatal(err)
		}
		other, err := experiment.ResolveReferenceTier(pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if err := g.Verify(context.Background(), other.Runtime); err != nil || verified != 1 || f.compiles != 0 || f.uploads != 0 {
			t.Fatal("shared medium runtime rejected or verification dispatched execution", err)
		}
	}
}

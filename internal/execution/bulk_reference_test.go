package execution

import (
	"bytes"
	"context"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestBulkTwentyOnePointTransferAndCleanup(t *testing.T) {
	ctx := context.Background()
	for _, tier := range []experiment.ReferenceTier{experiment.ReferenceMedium, experiment.ReferenceMediumBulk} {
		f := &cacheFixture{t: t, data: map[string][]byte{}, reference: true, referenceOutput: 4 << 20}
		c, _ := experiment.ResolveReferenceTier(tier)
		g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f, tier)
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
		completed := 0
		for i := 0; i < 21; i++ {
			size := 5 << 20
			if i == 0 {
				size = 16 << 20
			}
			out, err := g.Run(ctx, c.Runtime, o.Artifact, bytes.Repeat([]byte{'i'}, size))
			if err != nil {
				if tier != experiment.ReferenceMedium || g.totalBytes < 240<<20 || g.totalBytes > 256<<20 {
					t.Fatal("unexpected transfer failure", i, err)
				}
				break
			}
			if !bytes.Equal(out.Stdout, f.stdout) || len(f.data) != 1 {
				t.Fatal("large input/point cleanup changed")
			}
			completed++
		}
		if err := g.Remove(ctx, o.Artifact); err != nil || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
			t.Fatal("bulk final cleanup", err)
		}
		if tier == experiment.ReferenceMediumBulk && (completed != 21 || g.operations != 133 || g.totalBytes <= 299443885 || g.totalBytes >= 512<<20) {
			t.Fatal("complete transfer envelope or operation accounting")
		}
		if tier == experiment.ReferenceMedium && completed == 21 {
			t.Fatal("old adapter cap widened")
		}
	}
}

func TestBulkAdapterExactPayloadCapAndFailureCleanup(t *testing.T) {
	ctx := context.Background()
	f := &cacheFixture{t: t, data: map[string][]byte{}, reference: true, referenceOutput: 4 << 20}
	c, _ := experiment.ResolveReferenceTier(experiment.ReferenceMediumBulk)
	g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f, experiment.ReferenceMediumBulk)
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
	// Download charges the requested collector maximum, even for tiny output.
	g.totalBytes = (512 << 20) - int64(c.Runtime.OutputBytes) - 1
	if _, err := g.Run(ctx, c.Runtime, o.Artifact, nil); err != nil || g.totalBytes != 512<<20 {
		t.Fatal("bulk exact cumulative cap", err)
	}
	if _, err := g.Run(ctx, c.Runtime, o.Artifact, []byte("x")); err == nil || f.runs != 1 || g.totalBytes != 512<<20 {
		t.Fatal("bulk cumulative cap bypassed")
	}
	if err := g.Remove(ctx, o.Artifact); err != nil || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
		t.Fatal("cap failure prevented cleanup", err)
	}
}

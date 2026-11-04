package main

import (
	"context"
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestConvolutionExpectedRejectionKeepsIndependentCleanupErrors(t *testing.T) {
	for _, cleanup := range []string{"none", "stdout", "input"} {
		t.Run(cleanup, func(t *testing.T) {
			f := &staticRangeSumProbeFixture{cleanup: cleanup}
			c, err := experiment.ResolveReferenceTier(experiment.ReferenceConvolution)
			if err != nil {
				t.Fatal(err)
			}
			g, err := execution.NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, &f.host, experiment.ReferenceConvolution)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if err := g.Verify(ctx, c.Runtime); err != nil {
				t.Fatal(err)
			}
			o, err := g.Compile(ctx, c.Runtime, "source")
			if err != nil {
				t.Fatal(err)
			}
			_, runErr := g.Run(ctx, c.Runtime, o.Artifact, []byte("input"))
			if runErr == nil || staticRangeSumExpectedStderrFailure(runErr) != (cleanup == "none") {
				t.Fatal("expected rejection hid point cleanup failure")
			}
			if staticRangeSumExpectedStderrFailure(errors.Join(runErr, context.Canceled)) {
				t.Fatal("expected rejection hid cancellation")
			}
			if err := g.Remove(ctx, o.Artifact); err != nil {
				t.Fatal(err)
			}
		})
	}
}

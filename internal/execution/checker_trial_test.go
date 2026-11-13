package execution

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestMediumCheckerTrialPreservesCompleteOutputAboveSmallCap(t *testing.T) {
	ctx := t.Context()
	tier := experiment.ReferenceMedium
	c, err := experiment.ResolveReferenceTier(tier)
	if err != nil {
		t.Fatal(err)
	}
	p := c.Runtime
	f := &cacheFixture{t: t, data: map[string][]byte{}, reference: true, referenceOutput: p.OutputBytes}
	g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f, tier)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Verify(ctx, p); err != nil {
		t.Fatal(err)
	}
	compiled, err := g.Compile(ctx, p, "source")
	if err != nil {
		t.Fatal(err)
	}
	f.stdout = bytes.Repeat([]byte("12 34\r\n"), 150000)
	got, err := g.RunCheckerTrial(ctx, p, compiled.Artifact, []byte("input"))
	if err != nil || got.OutputTruncated || !bytes.Equal(got.Stdout, f.stdout) || len(f.data) != 1 {
		t.Fatal("complete medium output or cleanup failed", err)
	}
	f.fail = "collector-OLE"
	got, err = g.RunCheckerTrial(ctx, p, compiled.Artifact, []byte("input"))
	if verdict, classifyErr := Classify(got, false); err != nil || classifyErr != nil || verdict != "OLE" ||
		!got.OutputTruncated || len(got.Stdout) != p.OutputBytes+1 {
		t.Fatal("medium cap+1 collector evidence rejected", err, classifyErr)
	}
	if err := g.Remove(ctx, compiled.Artifact); err != nil || len(f.data) != 0 {
		t.Fatal("owned cleanup failed", err)
	}
}

func TestCheckerTrialCachedRunAcceptsExactReceiptAndFencesAmbiguousOverflow(t *testing.T) {
	ctx := t.Context()
	p := experiment.PinnedReferenceEnvironment()
	f := &cacheFixture{t: t, data: map[string][]byte{}, reference: true}
	g := NewReferenceGoJudge(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f)
	if err := g.Verify(ctx, p); err != nil {
		t.Fatal(err)
	}
	c, err := g.Compile(ctx, p, "source")
	if err != nil {
		t.Fatal(err)
	}
	f.fail = "collector-OLE"
	o, err := g.RunCheckerTrial(ctx, p, c.Artifact, []byte("input"))
	if verdict, e := Classify(o, false); err != nil || e != nil || verdict != "OLE" ||
		!o.OutputTruncated || len(o.Stdout) != p.OutputBytes+1 || len(f.data) != 1 {
		t.Fatal("trusted overflow or point cleanup failed", err, e)
	}
	f.fail = "OLE"
	if _, err := g.RunCheckerTrial(ctx, p, c.Artifact, []byte("input")); err == nil {
		t.Fatal("incomplete receipt was accepted")
	}
	calls := f.runs
	f.fail = ""
	if _, err := g.RunCheckerTrial(ctx, p, c.Artifact, []byte("input")); err == nil || f.runs != calls {
		t.Fatal("contradictory receipt did not fence reuse")
	}
	if err := g.Remove(ctx, c.Artifact); err != nil || len(f.data) != 0 {
		t.Fatal("owned cleanup failed after fencing", err)
	}
}

func TestCheckerTrialOutputRequiresCompleteOverflowEvidence(t *testing.T) {
	p := experiment.PinnedReferenceEnvironment()
	receipt := func() goJudgeResult {
		stdout, stderr, exit, cpu, memory := strings.Repeat("x", p.OutputBytes+1), "", 0, p.Run.CPU, p.Run.Memory
		r := goJudgeResult{Status: "Output Limit Exceeded", Error: "Output Limit Exceeded", Exit: &exit, CPU: &cpu, Memory: &memory,
			Files: map[string]*string{"stdout": &stdout, "stderr": &stderr}}
		r.FileErrors = append(r.FileErrors, struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}{"stdout", "CollectSizeExceeded"})
		return r
	}
	for _, tc := range []struct {
		name   string
		mutate func(*goJudgeResult)
		want   bool
	}{
		{"exact collector", func(*goJudgeResult) {}, true},
		{"unknown error", func(r *goJudgeResult) { r.Error = "unknown" }, false},
		{"no error label", func(r *goJudgeResult) { r.Error = "" }, false},
		{"accepted oversized", func(r *goJudgeResult) { r.Status = "Accepted"; r.Error = ""; r.FileErrors = nil }, false},
		{"negative CPU", func(r *goJudgeResult) { *r.CPU = -1 }, false},
		{"missing memory", func(r *goJudgeResult) { r.Memory = nil }, false},
		{"excess memory", func(r *goJudgeResult) { *r.Memory = p.Run.Memory + 1 }, false},
		{"nonzero exit", func(r *goJudgeResult) { *r.Exit = 1 }, false},
		{"short prefix", func(r *goJudgeResult) { *r.Files["stdout"] = "x" }, false},
		{"stderr collector", func(r *goJudgeResult) { r.FileErrors[0].Name = "stderr" }, false},
		{"copy failure", func(r *goJudgeResult) { r.FileErrors[0].Type = "CopyOutOpen" }, false},
		{"duplicate errors", func(r *goJudgeResult) { r.FileErrors = append(r.FileErrors, r.FileErrors[0]) }, false},
		{"unexpected role", func(r *goJudgeResult) { r.IDs = map[string]string{"other": "object"} }, false},
		{"missing stderr", func(r *goJudgeResult) { delete(r.Files, "stderr") }, false},
		{"exact file limit", func(r *goJudgeResult) {
			r.Error, r.FileErrors, *r.Exit = "", nil, 25
			*r.Files["stdout"] = (*r.Files["stdout"])[:p.OutputBytes]
		}, true},
		{"short file limit", func(r *goJudgeResult) {
			r.Error, r.FileErrors, *r.Exit = "", nil, 25
			*r.Files["stdout"] = "x"
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := receipt()
			tc.mutate(&r)
			if got := checkerTrialOutputEvidence(r, p); got != tc.want {
				t.Fatalf("evidence allowed=%t, want %t", got, tc.want)
			}
		})
	}
}

package execution

import (
	"context"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestCachedC17ProfileCannotWidenLegacyExecutor(t *testing.T) {
	f := &cacheFixture{t: t, data: map[string][]byte{}}
	base := experiment.PinnedReferenceEnvironment()
	c17 := experiment.C17Profile(base)
	for _, tc := range []struct {
		name    string
		enabled bool
		change  func(*experiment.ExecutionProfile)
	}{
		{name: "legacy runner"},
		{name: "changed command", enabled: true, change: func(p *experiment.ExecutionProfile) { p.CompileArgs[1] = "-std=gnu17" }},
		{name: "changed budget", enabled: true, change: func(p *experiment.ExecutionProfile) { p.Compile.Wall++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := c17
			p.CompileArgs = append([]string(nil), p.CompileArgs...)
			if tc.change != nil {
				tc.change(&p)
			}
			hostVerified := false
			g := NewReferenceGoJudge(f, func(context.Context, experiment.ExecutionProfile) error {
				hostVerified = true
				return nil
			}, f)
			g.c17 = tc.enabled
			if err := g.Verify(t.Context(), p); err == nil || hostVerified || f.uploads != 0 || !g.failed {
				t.Fatal("unsupported C17 profile was not fenced before uploading source")
			}
		})
	}
}

package execution

import (
	"context"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestCachedPublishedProfileCannotWidenLegacyExecutor(t *testing.T) {
	f := &cacheFixture{t: t, data: map[string][]byte{}}
	base := experiment.PinnedReferenceEnvironment()
	for _, tc := range []struct {
		name    string
		enabled bool
		profile experiment.ExecutionProfile
		change  func(*experiment.ExecutionProfile)
	}{
		{name: "legacy runner C17", profile: experiment.C17Profile(base)},
		{name: "legacy runner GNU++20", profile: experiment.GNUCPP20Profile(base)},
		{name: "legacy runner Java21", enabled: true, profile: experiment.Java21Profile(experiment.JavaTextBase(base))},
		{name: "changed command", enabled: true, profile: experiment.GNUCPP20Profile(base), change: func(p *experiment.ExecutionProfile) { p.CompileArgs[1] = "-std=gnu++23" }},
		{name: "changed budget", enabled: true, profile: experiment.C17Profile(base), change: func(p *experiment.ExecutionProfile) { p.Compile.Wall++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.profile
			p.CompileArgs = append([]string(nil), p.CompileArgs...)
			if tc.change != nil {
				tc.change(&p)
			}
			hostVerified := false
			g := NewReferenceGoJudge(f, func(context.Context, experiment.ExecutionProfile) error {
				hostVerified = true
				return nil
			}, f)
			g.publishedText = tc.enabled
			if err := g.Verify(t.Context(), p); err == nil || hostVerified || f.uploads != 0 || !g.failed {
				t.Fatal("unsupported profile was not fenced before uploading source")
			}
		})
	}
}

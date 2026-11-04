package experiment

import (
	"reflect"
	"testing"
)

func TestStaticRangeSumKeepsMediumIsolationAndOtherLimits(t *testing.T) {
	medium, err := ResolveReferenceTier(ReferenceMedium)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ResolveReferenceTier(ReferenceStaticRangeSum)
	if err != nil || ReferenceStaticRangeSum != 6 || c.Runtime.OutputBytes != 8<<20 || c.Files.PayloadBytes != 512<<20 || c.Entry != "" {
		t.Fatal("source-specific reference configuration", err)
	}
	if c.Runtime.ID == medium.Runtime.ID || c.Files.ID == medium.Files.ID {
		t.Fatal("new source borrowed medium identity")
	}
	unchangedRuntime, unchangedFiles := c.Runtime, c.Files
	unchangedRuntime.ID, unchangedRuntime.OutputBytes = medium.Runtime.ID, medium.Runtime.OutputBytes
	unchangedFiles.ID, unchangedFiles.PayloadBytes = medium.Files.ID, medium.Files.PayloadBytes
	if !reflect.DeepEqual(unchangedRuntime, medium.Runtime) || unchangedFiles != medium.Files {
		t.Fatal("output admission changed isolation, execution or other file limits")
	}
	for _, tier := range []ReferenceTier{0, 3, 5, 255} {
		if _, err := ResolveReferenceTier(tier); err == nil {
			t.Fatal("unadmitted tier accepted", tier)
		}
	}
	c.Runtime.SandboxCmd[0], c.Runtime.CompileArgs[0] = "modified", "modified"
	again, err := ResolveReferenceTier(ReferenceStaticRangeSum)
	if err != nil || again.Runtime.SandboxCmd[0] == "modified" || again.Runtime.CompileArgs[0] == "modified" {
		t.Fatal("shared mutable reference configuration", err)
	}
}

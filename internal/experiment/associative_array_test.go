package experiment

import (
	"reflect"
	"testing"
)

func TestAssociativeArrayKeepsExistingIsolationAndFileCaps(t *testing.T) {
	old, err := ResolveReferenceTier(ReferenceStaticRangeSum)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ResolveReferenceTier(ReferenceAssociativeArray)
	if err != nil || ReferenceAssociativeArray != 7 || c.Runtime.OutputBytes != 8<<20 || c.Files.PayloadBytes != 1<<30 || c.Entry != "" {
		t.Fatal("source-specific reference configuration", err)
	}
	if c.Runtime.Run != (Limits{CPU: 5000000000, Wall: 10000000000, Memory: 128 << 20, Processes: 16}) || c.Runtime.ID == old.Runtime.ID || c.Files.ID == old.Files.ID {
		t.Fatal("source runtime or identity")
	}
	unchanged, files := c.Runtime, c.Files
	unchanged.ID, unchanged.Run = old.Runtime.ID, old.Runtime.Run
	files.ID, files.PayloadBytes = old.Files.ID, old.Files.PayloadBytes
	if !reflect.DeepEqual(unchanged, old.Runtime) || files != old.Files {
		t.Fatal("source admission changed compiler, isolation, diagnostics or file/live caps")
	}
	c.Runtime.SandboxCmd[0], c.Runtime.CompileArgs[0] = "modified", "modified"
	again, err := ResolveReferenceTier(ReferenceAssociativeArray)
	if err != nil || again.Runtime.SandboxCmd[0] == "modified" || again.Runtime.CompileArgs[0] == "modified" {
		t.Fatal("shared source configuration")
	}
}

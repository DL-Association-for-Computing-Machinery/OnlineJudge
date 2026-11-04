package experiment

import (
	"reflect"
	"testing"
)

func TestPrimalityTierHasIndependentIdentityAndBoundedCompilation(t *testing.T) {
	medium, _ := ResolveReferenceTier(ReferenceMedium)
	p, err := ResolveReferenceTier(ReferencePrimality)
	if err != nil || ReferencePrimality != 4 || p.Entry != "sol/correct.cpp" || p.Runtime.ID == medium.Runtime.ID || p.Files.ID == medium.Files.ID || p.Runtime.OutputBytes != 512<<10 || p.Files.Objects != 6 || p.Files.PayloadBytes != 256<<20 {
		t.Fatal("dependency tier not explicitly bound")
	}
	if !reflect.DeepEqual(p.Runtime.SandboxArgs, medium.Runtime.SandboxArgs) || !reflect.DeepEqual(p.Runtime.SandboxCmd, medium.Runtime.SandboxCmd) || p.Runtime.Run != medium.Runtime.Run || p.Files.LiveBytes != medium.Files.LiveBytes {
		t.Fatal("outer configuration or execution resources changed")
	}
	p.Runtime.CompileArgs[4] = "changed"
	again, _ := ResolveReferenceTier(ReferencePrimality)
	if again.Runtime.CompileArgs[4] != "sol/correct.cpp" {
		t.Fatal("mutable compile args shared")
	}
	if _, err := ResolveReferenceTier(5); err == nil {
		t.Fatal("unadmitted tier accepted")
	}
}

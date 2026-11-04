package experiment

import (
	"reflect"
	"testing"
)

func TestConvolutionConfigurationKeepsIsolationAndFreshSlices(t *testing.T) {
	old, err := ResolveReferenceTier(ReferenceAssociativeArray)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ResolveReferenceTier(ReferenceConvolution)
	if err != nil || ReferenceConvolution != 8 || c.Entry != "" || c.Runtime.ID != "p4-convolution-gnu17-desktop-arm64-v1" || c.Files.ID != "p4-gojudge-cached-convolution-v1" || c.Runtime.OutputBytes != 16<<20 || c.Files.PayloadBytes != 2<<30 {
		t.Fatal("convolution configuration identity or output/payload bounds", err)
	}
	if c.Runtime.Run != (Limits{CPU: 5000000000, Wall: 10000000000, Memory: 128 << 20, Processes: 16}) || c.Files.FileBytes != 48<<20 || c.Files.LiveBytes != 64<<20 || c.Files.Objects != 4 || c.Files.Operations != 512 {
		t.Fatal("convolution changed execution or file bounds")
	}
	unchanged, files := c.Runtime, c.Files
	unchanged.ID, unchanged.OutputBytes = old.Runtime.ID, old.Runtime.OutputBytes
	unchanged.SandboxCmd = append([]string(nil), c.Runtime.SandboxCmd...)
	for i := 0; i+1 < len(unchanged.SandboxCmd); i++ {
		if unchanged.SandboxCmd[i] == "-output-limit" {
			if unchanged.SandboxCmd[i+1] != "16777216" {
				t.Fatal("convolution physical output limit is not 16MiB")
			}
			unchanged.SandboxCmd[i+1] = "8388608"
		}
	}
	files.ID, files.PayloadBytes = old.Files.ID, old.Files.PayloadBytes
	if !reflect.DeepEqual(unchanged, old.Runtime) || files != old.Files {
		t.Fatal("convolution changed compiler, isolation, diagnostics or unrelated file caps")
	}
	c.Runtime.SandboxCmd[0], c.Runtime.CompileArgs[0] = "changed", "changed"
	again, err := ResolveReferenceTier(ReferenceConvolution)
	if err != nil || again.Runtime.SandboxCmd[0] == "changed" || again.Runtime.CompileArgs[0] == "changed" {
		t.Fatal("convolution configuration shares mutable slices", err)
	}
}

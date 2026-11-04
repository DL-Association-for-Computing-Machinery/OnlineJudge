package experiment

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestReferenceTierClosedFreshConfiguration(t *testing.T) {
	for _, tier := range []ReferenceTier{0, 5, 255} {
		if _, err := ResolveReferenceTier(tier); err == nil {
			t.Fatal("unknown tier accepted", tier)
		}
	}
	small, err := ResolveReferenceTier(ReferenceSmall)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(small.Runtime, PinnedReferenceEnvironment()) || small.Files != PinnedReferenceFiles() {
		t.Fatal("small identity changed")
	}
	medium, err := ResolveReferenceTier(ReferenceMedium)
	if err != nil {
		t.Fatal(err)
	}
	if medium.Files.PayloadBytes != 256<<20 || medium.Runtime.OutputBytes != 4<<20 || !reflect.DeepEqual(medium.Runtime.SandboxArgs, []string{"/opt/go-judge"}) {
		t.Fatal("medium bounds")
	}
	bulk, err := ResolveReferenceTier(ReferenceMediumBulk)
	if err != nil || !reflect.DeepEqual(bulk.Runtime, medium.Runtime) {
		t.Fatal("bulk changed runtime", err)
	}
	wantFiles := medium.Files
	wantFiles.ID, wantFiles.PayloadBytes = "p4-gojudge-cached-files-medium-bulk-v1", 512<<20
	if bulk.Files != wantFiles {
		t.Fatal("bulk widened unrelated file bounds")
	}
	bulk.Runtime.CompileArgs[0] = "modified"
	bulkAgain, _ := ResolveReferenceTier(ReferenceMediumBulk)
	if !reflect.DeepEqual(bulkAgain.Runtime, medium.Runtime) {
		t.Fatal("bulk shares mutable runtime")
	}
	args := medium.Runtime.SandboxCmd
	output, copyOut := "", ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-output-limit" {
			output = args[i+1]
		}
		if args[i] == "-copy-out-limit" {
			copyOut = args[i+1]
		}
	}
	if output != "8388608" || copyOut != "1048576" {
		t.Fatal("global output/copy budgets mixed")
	}
	medium.Runtime.SandboxCmd[0] = "modified"
	medium.Runtime.CompileArgs[0] = "modified"
	again, _ := ResolveReferenceTier(ReferenceMedium)
	if again.Runtime.SandboxCmd[0] == "modified" || again.Runtime.CompileArgs[0] == "modified" {
		t.Fatal("shared medium configuration")
	}
	for _, p := range []ExecutionProfile{PinnedEnvironment(), PinnedReferenceEnvironment()} {
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, ok := fields["sandbox_cmd"]; ok {
			t.Fatal("old serialized profile changed")
		}
	}
}

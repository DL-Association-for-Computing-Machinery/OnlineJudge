package problemvalidation_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

type dependentSandbox struct {
	sandbox
	sourceFiles []execution.SourceFile
}

func (s *dependentSandbox) CompileFiles(ctx context.Context, p experiment.ExecutionProfile, files []execution.SourceFile) (execution.Observation, error) {
	s.calls = append(s.calls, "compile-files")
	s.sourceFiles = files
	return s.compile, nil
}

func TestPrimalityRunnerRequiresFrozenDependenciesBeforeExecution(t *testing.T) {
	for _, mode := range []string{"pass", "missing-capability", "bad-header", "bad-last-answer"} {
		t.Run(mode, func(t *testing.T) {
			b := problemfixture.NewPrimality(t)
			answer := []byte("5\n")
			if mode == "bad-last-answer" {
				answer = []byte{255, '\n'}
			}
			_, prepared := referenceRunnerFixture(t, b, answer)
			profile, tier, err := problemvalidation.ResolveSource("library-checker:primality_test")
			if err != nil || tier != experiment.ReferencePrimality {
				t.Fatal(err)
			}
			s := &dependentSandbox{sandbox: sandbox{runtime: profile.Runtime, compile: accepted(""), run: []execution.Observation{accepted("3"), accepted("5")}}}
			s.compile.Artifact = "compiled"
			if mode == "bad-header" {
				ref := b.Document["provenance"].(map[string]any)["original_files"].(map[string]any)["source/common/random.h"].(map[string]any)
				raw := append([]byte(nil), b.Files[ref["path"].(string)]...)
				raw[0] ^= 255
				if err := os.WriteFile(filepath.Join(b.Directory, ref["path"].(string)), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			var adapter execution.Sandbox = s
			if mode == "missing-capability" {
				adapter = &s.sandbox
			}
			r, err := problemvalidation.New(adapter).ValidateSource(context.Background(), prepared, "library-checker:primality_test")
			if mode != "pass" {
				if err == nil || len(s.calls) != 0 {
					t.Fatal("unsupported dependencies or bad material reached Verify")
				}
				return
			}
			if err != nil || r.Verdict != "AC" || len(s.sourceFiles) != 5 || len(r.Plan.Dependencies) != 4 || r.Plan.SourcePath != "sol/correct.cpp" || s.calls[1] != "compile-files" {
				t.Fatal("complete dependency runner", err)
			}
			if err := r.Finalize(nil, nil, nil); err != nil || r.Publication != "blocked" || r.Status != "reference-validation-passed" {
				t.Fatal("publication/stop gate")
			}
		})
	}
}

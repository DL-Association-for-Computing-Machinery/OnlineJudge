//go:build sandbox

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestPrimalityReferenceSandbox(t *testing.T) {
	if os.Getenv("OJ_PRIMALITY_REFERENCE") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv("OJ_WORKER_M0_SLOT") != "" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("explicit independent Desktop primality experiment required")
	}
	if !t.Run("nested-compilation-layout", func(t *testing.T) {
		h := referenceTierSandboxHost(t, experiment.ReferencePrimality)
		c, err := experiment.ResolveReferenceTier(experiment.ReferencePrimality)
		if err != nil {
			t.Fatal(err)
		}
		g, err := execution.NewReferenceGoJudgeForTier(h, h.Verify, h, experiment.ReferencePrimality)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := g.Verify(ctx, c.Runtime); err != nil {
			t.Fatal(err)
		}
		// Fixed trusted fixtures prove directory creation and original relative
		// include resolution. Upstream code runs only in the next full CLI group.
		files := []execution.SourceFile{
			{Name: "sol/correct.cpp", Data: []byte("#include <cstdio>\n#include \"../lib/prime.hpp\"\nint main(){printf(\"%d\\n\",value());}\n")},
			{Name: "lib/prime.hpp", Data: []byte("#include \"montgomery.hpp\"\n")},
			{Name: "lib/montgomery.hpp", Data: []byte("#include \"util.hpp\"\n")},
			{Name: "lib/util.hpp", Data: []byte("#include \"random.h\"\n")},
			{Name: "common/random.h", Data: []byte("// 保留 UTF-8 原始字节\ninline int value(){return 3;}\n")},
		}
		o, err := g.CompileFiles(ctx, c.Runtime, files)
		compileVerdict, compileErr := execution.Classify(o, true)
		if err != nil || compileErr != nil || compileVerdict != "AC" || o.Artifact == "" {
			t.Fatal("nested layout compile", execution.FailureDetails(err))
		}
		out, err := g.Run(ctx, c.Runtime, o.Artifact, nil)
		verdict, classifyErr := execution.Classify(out, false)
		if err != nil || classifyErr != nil || verdict != "AC" || string(out.Stdout) != "3\n" {
			t.Fatal("nested relative include execution")
		}
		if err := g.Remove(ctx, o.Artifact); err != nil {
			t.Fatal(err)
		}
		t.Log("five cached source files, nested paths and -Icommon compiled and ran under the fixed runtime")
	}) {
		return
	}
	t.Run("complete-thirteen-point-cli", func(t *testing.T) {
		referenceSourceCLI(t, "library-checker:primality_test", "OJ_PRIMALITY_OUTPUT", 13)
		raw, err := os.ReadFile(filepath.Join(os.Getenv("OJ_PRIMALITY_OUTPUT"), "report.json"))
		if err != nil {
			t.Fatal(err)
		}
		var report problemvalidation.Report
		if err := json.Unmarshal(raw, &report); err != nil {
			t.Fatal(err)
		}
		if report.Plan.SourcePath != "sol/correct.cpp" || len(report.Plan.Dependencies) != 4 || report.Plan.Source.SHA256 != "714746e38b42e97ce8cad7dbb5ef80df4ff39094cf62d3f4945ebb82a2f49e3b" {
			t.Fatal("real compilation identity/layout not bound")
		}
		want := []string{"328682e1a024b784f8b524d84057e72c2a93b8589c14903f569af53f4ad033d2", "4c87166120c97435b89d8c398790f95a3bd586773f16a3e41b4f3b1b6b8c37a5", "3e83df19dc6937735610a69fea20f67e203c0241b7e4b062647b33795f967101", "d0ae9470c3a04425e1ab8c39f376a0e707507659b8f5aa3ca06c5b47f193613c"}
		for i, dependency := range report.Plan.Dependencies {
			if dependency.Object.SHA256 != want[i] {
				t.Fatal("upstream dependency hash differs")
			}
		}
	})
}

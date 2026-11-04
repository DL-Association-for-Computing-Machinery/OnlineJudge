//go:build sandbox

package main

import (
	"os"
	"runtime"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestConvolutionReferenceSandbox(t *testing.T) {
	if os.Getenv("OJ_CONVOLUTION_REFERENCE") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv("OJ_WORKER_M0_SLOT") != "" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("explicit independent Desktop convolution experiment required")
	}
	if !t.Run("raw-sixteen-MiB-and-narrow-output-evidence", func(t *testing.T) {
		testReferenceOutputBoundary(t, experiment.ReferenceConvolution, 16<<20, "convolution")
	}) {
		return
	}
	t.Run("complete-fifty-three-point-cli", func(t *testing.T) {
		referenceSourceCLI(t, "library-checker:convolution_mod", "OJ_CONVOLUTION_OUTPUT", 53)
	})
}

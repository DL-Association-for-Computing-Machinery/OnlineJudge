//go:build sandbox

package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

// This test-only program changes one valid field element on the hand-checked
// case. It is compiled by the fixed sandbox, not admitted by the Worker CLI.
const wrongConvolution = `#include <cstdio>
int main() {
    int n, m;
    if (scanf("%d%d", &n, &m) != 2 || n < 1 || n > 16 || m < 1 || m > 16) return 1;
    long long a[16], b[16], c[31] = {};
    const long long mod = 998244353;
    for (int i=0; i<n; ++i) if (scanf("%lld", &a[i]) != 1) return 1;
    for (int i=0; i<m; ++i) if (scanf("%lld", &b[i]) != 1) return 1;
    for (int i=0; i<n; ++i)
        for (int j=0; j<m; ++j) c[i+j] = (c[i+j] + a[i]*b[j]) % mod;
    if (n==2 && m==2 && a[0]==1 && a[1]==2 && b[0]==3 && b[1]==4) c[0]=(c[0]+1)%mod;
    for (int i=0; i<n+m-1; ++i) printf("%lld%c", c[i], i==n+m-2 ? '\n' : ' ');
}`

type convolutionNegativeAdapter struct{ execution.Sandbox }

func (a convolutionNegativeAdapter) Compile(ctx context.Context, p experiment.ExecutionProfile, _ string) (execution.Observation, error) {
	return a.Sandbox.Compile(ctx, p, wrongConvolution)
}

func TestConvolutionDifferentialNegativeSandbox(t *testing.T) {
	if os.Getenv("OJ_CONVOLUTION_DIFFERENTIAL_TEST") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" ||
		os.Getenv(dualModeEnv) != "" || os.Getenv("OJ_WORKER_M0_SLOT") != "" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("explicit dedicated Desktop convolution differential experiment required")
	}
	root := os.Getenv("OJ_CONVOLUTION_DIFFERENTIAL_SOURCE_ROOT")
	lockPath := os.Getenv("OJ_CONVOLUTION_DIFFERENTIAL_SOURCE_LOCK")
	out := os.Getenv("OJ_CONVOLUTION_DIFFERENTIAL_TEST_OUTPUT")
	if !canonicalAbsolute(root) || !canonicalAbsolute(lockPath) || !canonicalAbsolute(out) || !referenceOutputOutside(root, out) {
		t.Fatal("absolute source and fresh report paths required")
	}
	lockBytes, err := readValidationLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := problemvalidation.LoadConvolutionSources(root, lockBytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "INCOMPLETE"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	slot, err := workerhost.ReferenceSlotForTier(experiment.ReferenceConvolution)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := lockSlotMode(slot.LockName(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	report, err := runConvolutionTrialLocked(t.Context(), sources, lock, func(ctx context.Context) (validationHost, error) {
		h, e := workerhost.StartReferenceSlot(ctx, slot)
		if h == nil {
			return nil, e
		}
		return h, e
	}, func(ctx context.Context, s problemvalidation.ConvolutionSources, factory func() (execution.Sandbox, error)) (problemvalidation.ConvolutionReport, error) {
		count := 0
		r, err := problemvalidation.RunSmallConvolution(ctx, s, func() (execution.Sandbox, error) {
			count++
			adapter, e := factory()
			if count == 2 && e == nil {
				return convolutionNegativeAdapter{adapter}, nil
			}
			return adapter, e
		})
		r.Schema = "oj.convolution-differential-negative-control.v1"
		r.Candidate = problemvalidation.ConvolutionProgram{Path: "synthetic-test-control.cpp", Size: int64(len(wrongConvolution)),
			SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(wrongConvolution)))}
		return r, err
	})
	if persistErr := persistPrivateValidationReport(out, report, report.Status != "incomplete"); persistErr != nil {
		t.Fatal(persistErr)
	}
	c := report.Counterexample
	if !errors.Is(err, problemvalidation.ErrConvolutionMismatch) || report.Status != "mismatch" || !report.StopConfirmed ||
		report.Publication != "blocked" || report.OracleParsed != 24 || report.CandidateEqual != 9 ||
		c == nil || c.Case != 10 || c.Input != "2 2\n1 2\n3 4\n" ||
		!slices.Equal(c.Expected, []uint32{3, 10, 8}) || !slices.Equal(c.Actual, []uint32{4, 10, 8}) {
		t.Fatal("expected isolated, determinate disagreement; inspect private report", err)
	}
}

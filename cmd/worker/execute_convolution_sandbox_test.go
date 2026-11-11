//go:build sandbox

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

type pauseConvolutionComparison struct {
	validationHost
	verifies int
	ready    chan struct{}
}

func (h *pauseConvolutionComparison) Verify(ctx context.Context, p experiment.ExecutionProfile) error {
	h.verifies++
	if h.verifies == 2 {
		close(h.ready)
		<-ctx.Done()
		return ctx.Err()
	}
	return h.validationHost.Verify(ctx, p)
}

// Runs only the fixed reviewed bundle on the existing, owned Desktop slot.
// The transport and database are independently tested with real mTLS and PG.
func TestClaimedConvolutionOwnedSandbox(t *testing.T) {
	if os.Getenv("OJ_CONVOLUTION_LEASED_SANDBOX_TEST") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" ||
		runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" || os.Getenv(dualModeEnv) != "" || os.Getenv("OJ_WORKER_M0_SLOT") != "" {
		t.Fatal("requires explicitly opted-in, fixed Desktop convolution sandbox")
	}
	bundle, source, lockPath := os.Getenv("OJ_CONVOLUTION_BUNDLE"), os.Getenv("OJ_CONVOLUTION_SOURCE_ROOT"), os.Getenv("OJ_CONVOLUTION_SOURCE_LOCK")
	if !canonicalAbsolute(bundle) || !canonicalAbsolute(source) || !canonicalAbsolute(lockPath) {
		t.Fatal("absolute bundle, source and reviewed lock paths required")
	}
	raw, err := readValidationLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	const id = "00000000000040008000000000000001"
	prepared, err := problem.LoadBundle(bundle, problemvalidation.ConvolutionSourceID, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := prepared.Close(); err != nil {
			t.Error(err)
		}
	}()
	snapshot, err := prepared.PrepareDiskReference(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := snapshot.Close(); err != nil {
			t.Error(err)
		}
	}()
	imported := snapshot.Plan()
	pinned, err := problemvalidation.LoadConvolutionSources(source, raw)
	if err != nil {
		t.Fatal(err)
	}
	meta := problemvalidation.NewConvolutionReport(pinned)
	profile, _, err := problemvalidation.ResolveSource(imported.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	comparison := problemvalidation.ConvolutionPlan{
		Protocol: problemvalidation.ConvolutionPlanProtocol, SourceID: meta.SourceID, SourceRevision: meta.SourceRevision,
		SourceLock: meta.SourceLock, Draft: problemvalidation.ConvolutionDraftBinding{ID: id, Version: imported.Version, ImportRevision: imported.ImportRevision},
		Oracle: meta.Oracle, Candidate: meta.Candidate, Corpus: meta.Corpus, Seed: meta.Seed, Cases: meta.Cases,
		CorpusSHA256:  "b4d0acee90e20829f5f79c65ee7eb4e5fc0d99308fe2b00387f930a0d2112576",
		ProfileSHA256: profile.Digest(), FormalSHA256: imported.Digest(),
	}
	frozen, err := comparison.FreezePrograms(t.Context(), imported.Digest(), func(_ context.Context, o materialstore.Object) (io.ReadCloser, error) {
		path := meta.Oracle.Path
		if o.SHA256 == meta.Candidate.SHA256 {
			path = meta.Candidate.Path
		}
		return os.Open(filepath.Join(source, path))
	})
	if err != nil {
		t.Fatal(err)
	}
	claim := problemreference.WorkerClaim{RequestID: id, SourceID: imported.SourceID,
		PlanSHA256: imported.Digest(), Token: strings.Repeat("a", 64), Until: time.Now().Add(30 * time.Second)}
	slot, err := referenceSlotForSource(imported.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := lockSlotMode(slot, false)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	reported := false
	fault := os.Getenv("OJ_CONVOLUTION_LEASE_LOSS") == "1"
	ready := make(chan struct{})
	err = runClaimedReference(t.Context(), lock, claim, nil, func(context.Context, problemreference.WorkerClaim) (referenceExecutionMaterials, error) {
		return referenceExecutionMaterials{snapshot: snapshot, close: snapshot.Close, comparison: &frozen}, nil
	}, func(context.Context, problemreference.WorkerClaim) error {
		select {
		case <-ready:
			return problemreference.ErrClaimUnavailable
		default:
			return nil
		}
	}, func(ctx context.Context, sourceID string) (validationHost, error) {
		host, err := startClaimedReference(ctx, sourceID)
		if fault && err == nil && host != nil {
			return &pauseConvolutionComparison{validationHost: host, ready: ready}, nil
		}
		return host, err
	},
		func(_ context.Context, input problemreference.WorkerReportInput) error {
			e, err := input.DecodeEvidence()
			if err != nil || reported || !e.StopConfirmed || e.Verdict != "AC" || len(e.Points) != 53 ||
				e.Comparison == nil || e.Comparison.PlanSHA256 != comparison.Digest() || e.Comparison.Status != "passed" || e.Comparison.Matched != 24 {
				t.Fatal("combined evidence did not prove both determinate results", err)
			}
			reported = true
			return nil
		})
	if fault {
		if reported || !errors.Is(err, execution.ErrLeaseLost) || errors.Is(err, execution.ErrStopUnconfirmed) {
			t.Fatal("lost comparison reservation did not stop without evidence", err)
		}
		select {
		case <-ready:
		default:
			t.Fatal("loss injection did not reach comparison")
		}
		t.Log("comparison renewal rejected after formal phase; stop confirmed, no report")
		return
	}
	if err != nil || !reported {
		t.Fatal("combined execution did not finish with stopped evidence", err)
	}
	t.Log("same owned sandbox: 53 formal AC, 24 matching comparisons, confirmed stop")
}

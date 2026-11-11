package problemvalidation_test

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func frozenRunnerSnapshot(t *testing.T, ctx context.Context, lock string) *problem.ReferenceSnapshot {
	t.Helper()
	bundle, prepared := fixture(t)
	plan, err := prepared.PrepareReference(ctx, problem.SmallReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	// Use the admitted identity with synthetic bytes only against a fake
	// sandbox; this test does not validate the upstream source lock inventory.
	plan.SourceLockSHA256 = lock
	plan.SourceData = nil
	for i := range plan.Cases {
		plan.Cases[i].InputData, plan.Cases[i].AnswerData = nil, nil
	}
	snapshot, err := plan.FreezeMaterials(ctx, func(_ context.Context, object materialstore.Object) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bundle.Files["objects/sha256/"+object.SHA256])), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := snapshot.Close(); err != nil {
			t.Error(err)
		}
	})
	return snapshot
}

func TestFrozenReferenceUsesExpectedPlanAndOwnerKeepsSnapshot(t *testing.T) {
	snapshot := frozenRunnerSnapshot(t, t.Context(), problemvalidation.SourceLockSHA256)
	plan := snapshot.Plan()
	s := &sandbox{compile: accepted(""), run: []execution.Observation{accepted("3\n"), accepted("5\n")}}
	s.compile.Artifact = "compiled"
	runner := problemvalidation.New(s)
	report, err := runner.ValidateFrozen(t.Context(), snapshot, plan.SourceID, plan.Digest())
	if err != nil || report.Verdict != "AC" || report.PlanSHA256 != plan.Digest() || report.StopConfirmed || report.Publication != "blocked" {
		t.Fatalf("invalid pre-stop result: %+v (%v)", report, err)
	}
	want := []string{"verify", "compile", "run:1 2\n", "run:2 3\n", "remove"}
	if !reflect.DeepEqual(s.calls, want) {
		t.Fatalf("frozen points executed out of order: %q", s.calls)
	}
	if _, err := snapshot.ReadSource(t.Context()); err != nil {
		t.Fatal("runner consumed the owner's snapshot lifetime", err)
	}
	if _, err := runner.ValidateFrozen(t.Context(), snapshot, plan.SourceID, plan.Digest()); err == nil || !reflect.DeepEqual(s.calls, want) {
		t.Fatal("single-use runner accepted a second execution", err)
	}
}

func TestFrozenReferenceMismatchAndCanceledSnapshotNeverReachSandbox(t *testing.T) {
	for _, kind := range []string{"plan", "source", "lock", "cancelled snapshot"} {
		t.Run(kind, func(t *testing.T) {
			lifetime, cancel := context.WithCancel(t.Context())
			defer cancel()
			lock := problemvalidation.SourceLockSHA256
			if kind == "lock" {
				lock = strings.Repeat("0", 64)
			}
			snapshot := frozenRunnerSnapshot(t, lifetime, lock)
			plan := snapshot.Plan()
			source, digest := plan.SourceID, plan.Digest()
			switch kind {
			case "plan":
				digest = strings.Repeat("a", 64)
			case "source":
				source = "library-checker:unionfind"
			case "cancelled snapshot":
				cancel()
			}
			s := &sandbox{}
			report, err := problemvalidation.New(s).ValidateFrozen(t.Context(), snapshot, source, digest)
			if err == nil || report.Verdict != "SE" || len(s.calls) != 0 {
				t.Fatalf("invalid frozen material reached sandbox: %+v, %v, %q", report, err, s.calls)
			}
		})
	}
}

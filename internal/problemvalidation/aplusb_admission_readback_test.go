package problemvalidation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
)

func writeObservation(t *testing.T, path string, record any) string {
	t.Helper()
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return handoffDigest(raw)
}

func TestAPlusBAdmissionReadbackJoinsThreePinnedInputs(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	receipt, wantedPin, worker, target := targetObservationFixture(t)
	in := APlusBAdmissionInputs{
		ReceiptDirectory: filepath.Join(base, "receipt"), HandoffSHA256: receipt.Readback.Handoff,
		TargetObservationPath: filepath.Join(base, "runtime.json"), CheckerObservationPath: filepath.Join(base, "checker.json"),
		WorkerArtifactSHA256: worker, Target: target.Target,
	}
	in.ReceiptSHA256, err = SealAPlusBHandoffReceipt(t.Context(), in.ReceiptDirectory, receipt.Readback)
	if err != nil || in.ReceiptSHA256 != wantedPin {
		t.Fatalf("seal fixture receipt: %v", err)
	}
	in.TargetObservationSHA256 = writeObservation(t, in.TargetObservationPath, target)
	checker := aplusbCheckerObservation{
		Schema: aplusbCheckerObservationSchema, Material: target.Material, Target: target.Target,
		WorkerArtifact: worker, CheckerVersion: judging.LibraryCheckerTextVersion, OutputBytes: 262144, TokenBytes: 12,
		TargetRecord: in.TargetObservationSHA256,
		Observed:     aplusbObservationProvenance{"test-checker", "2026-11-14T03:05:00Z", strings.Repeat("d", 64)},
		ReferenceOK:  true, WrongOutputOK: true, BoundedCapture: true, FaultsChecked: true,
	}
	in.CheckerObservationSHA256 = writeObservation(t, in.CheckerObservationPath, checker)
	got, err := ReadAPlusBAdmissionEvidence(t.Context(), in)
	if err != nil || got.Scope != "review-input-only" || got.ImportRevision != receipt.ImportRevision ||
		got.FormalPlanSHA256 != ReviewedAPlusBPlan || got.CandidateSHA256 != ReviewedAPlusBCandidate ||
		got.Target != in.Target || got.WorkerArtifactSHA256 != worker || got.ReceiptSHA256 != wantedPin ||
		got.HandoffSHA256 != in.HandoffSHA256 || got.TargetObservationSHA256 != in.TargetObservationSHA256 ||
		got.CheckerObservationSHA256 != in.CheckerObservationSHA256 {
		t.Fatalf("readback lost a binding: %v", err)
	}
	for _, tc := range []struct {
		name string
		edit func(*APlusBAdmissionInputs)
	}{
		{"missing receipt", func(i *APlusBAdmissionInputs) { i.ReceiptDirectory = base }},
		{"missing target", func(i *APlusBAdmissionInputs) { i.TargetObservationPath = filepath.Join(base, "missing") }},
		{"missing checker", func(i *APlusBAdmissionInputs) { i.CheckerObservationPath = filepath.Join(base, "missing") }},
		{"missing external worker pin", func(i *APlusBAdmissionInputs) { i.WorkerArtifactSHA256 = "" }},
		{"wrong independently selected worker", func(i *APlusBAdmissionInputs) { i.WorkerArtifactSHA256 = strings.Repeat("9", 64) }},
		{"checker file used as target", func(i *APlusBAdmissionInputs) {
			i.TargetObservationPath, i.TargetObservationSHA256 = i.CheckerObservationPath, i.CheckerObservationSHA256
		}},
		{"target file used as checker", func(i *APlusBAdmissionInputs) {
			i.CheckerObservationPath, i.CheckerObservationSHA256 = i.TargetObservationPath, i.TargetObservationSHA256
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := in
			tc.edit(&bad)
			if got, err := ReadAPlusBAdmissionEvidence(t.Context(), bad); err == nil || got != (APlusBAdmissionReadback{}) {
				t.Fatal("readback returned partial or unverified data")
			}
		})
	}
	for _, tc := range []struct {
		name string
		edit func(*aplusbCheckerObservation)
	}{
		{"different candidate", func(o *aplusbCheckerObservation) { o.Material.Candidate = strings.Repeat("9", 64) }},
		{"different plan", func(o *aplusbCheckerObservation) { o.Material.FormalPlan = strings.Repeat("9", 64) }},
		{"different worker", func(o *aplusbCheckerObservation) { o.WorkerArtifact = strings.Repeat("9", 64) }},
		{"another target observation", func(o *aplusbCheckerObservation) { o.TargetRecord = strings.Repeat("9", 64) }},
		{"shared underlying observation", func(o *aplusbCheckerObservation) { o.Observed.Evidence = target.Observed.Evidence }},
		{"raw trial report used as checker observation", func(o *aplusbCheckerObservation) { o.Observed.Evidence = target.Reports[1] }},
		{"different semantics", func(o *aplusbCheckerObservation) { o.CheckerVersion = "other" }},
		{"widened byte limit", func(o *aplusbCheckerObservation) { o.OutputBytes++ }},
		{"widened token limit", func(o *aplusbCheckerObservation) { o.TokenBytes++ }},
		{"reference not checked", func(o *aplusbCheckerObservation) { o.ReferenceOK = false }},
		{"wrong output not checked", func(o *aplusbCheckerObservation) { o.WrongOutputOK = false }},
		{"capture not checked", func(o *aplusbCheckerObservation) { o.BoundedCapture = false }},
		{"fault classification not checked", func(o *aplusbCheckerObservation) { o.FaultsChecked = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			edit := checker
			tc.edit(&edit)
			bad := in
			bad.CheckerObservationPath = filepath.Join(base, "changed.json")
			bad.CheckerObservationSHA256 = writeObservation(t, bad.CheckerObservationPath, edit)
			if got, err := ReadAPlusBAdmissionEvidence(t.Context(), bad); err == nil || got != (APlusBAdmissionReadback{}) {
				t.Fatal("externally pinned but mismatched checker record accepted")
			}
		})
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := ReadAPlusBAdmissionEvidence(cancelled, in); err == nil || got != (APlusBAdmissionReadback{}) {
		t.Fatal("cancelled readback returned an observation")
	}
}

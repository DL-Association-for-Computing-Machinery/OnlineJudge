package problemvalidation_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestVerifyPassingCheckerTrial(t *testing.T) {
	_, p := fixture(t)
	ctx := t.Context()
	plan, err := p.PrepareReference(ctx, problem.SmallReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	limits := judging.TextLimits{Bytes: experiment.PinnedReferenceEnvironment().OutputBytes, TokenBytes: 12}
	c, err := problemvalidation.PreflightCheckerCandidate(ctx, p, plan.Digest(), judging.LibraryCheckerTextVersion, limits)
	if err != nil {
		t.Fatal(err)
	}
	trial, err := problemvalidation.PrepareCheckerTrial(ctx, p, plan.Digest(), c.Digest(), limits)
	if err != nil {
		t.Fatal(err)
	}
	// Start with the producer's report shape and fill synthetic accepted
	// observations. No sandbox or actual trial pass is claimed by this fixture.
	base, err := trial.Run(ctx, execution.NewCachedGoJudge(nil, nil, nil))
	if err == nil || len(base.Points) != 2 {
		t.Fatalf("expected two provisional points and an unavailable sandbox: %+v, %v", base, err)
	}
	const slot = "oj-reference-sandbox"
	base.Slot, base.Verdict, base.Diagnostic = slot, "AC", nil
	base.Complete, base.StopConfirmed = true, true
	base.CompileUsage = &execution.Usage{CPU: base.RuntimeProfile.Runtime.Compile.CPU}
	for i := range base.Points {
		base.Points[i].Verdict = "AC"
		base.Points[i].Usage = &execution.Usage{Memory: base.RuntimeProfile.Runtime.Run.Memory}
	}
	marshal := func(r problemvalidation.CheckerTrialReport) []byte {
		t.Helper()
		raw, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return append(raw, '\n')
	}
	digest := func(raw []byte) string {
		sum := sha256.Sum256(raw)
		return hex.EncodeToString(sum[:])
	}
	raw := marshal(base)
	got, err := trial.VerifyPassingReport(raw, digest(raw), slot)
	if err != nil || got.ReportSHA256 != digest(raw) || got.CandidateSHA256 != c.Digest() ||
		got.FormalPlanSHA != plan.Digest() || got.RuntimeSHA256 != base.RuntimeSHA256 || got.Points != len(plan.Cases) {
		t.Fatalf("verified receipt does not match trial: %+v, %v", got, err)
	}
	reject := func(t *testing.T, data []byte, pinned string) {
		t.Helper()
		got, err := trial.VerifyPassingReport(data, pinned, slot)
		if !errors.Is(err, problemvalidation.ErrCheckerTrialEvidence) || got != (problemvalidation.CheckerTrialVerification{}) {
			t.Fatalf("unverified evidence returned a receipt: %+v, %v", got, err)
		}
	}
	reject(t, raw, strings.Repeat("1", 64))
	for _, data := range [][]byte{
		bytes.TrimSpace(raw),
		bytes.Replace(raw, []byte("\"complete\": true"), []byte("\"complete\": false,\n  \"complete\": true"), 1),
		bytes.Replace(raw, []byte("\"complete\": true"), []byte("\"complete\": true,\n  \"unknown\": 1"), 1),
	} {
		reject(t, data, digest(data))
	}
	for _, tc := range []struct {
		name   string
		change func(*problemvalidation.CheckerTrialReport)
	}{
		{"wrong schema", func(r *problemvalidation.CheckerTrialReport) { r.Schema = "oj.checker-trial.v2" }},
		{"isolated judge schema", func(r *problemvalidation.CheckerTrialReport) { r.Schema = "oj.aplusb-isolated-judge-report.v1" }},
		{"legacy schema carrying a submission binding", func(r *problemvalidation.CheckerTrialReport) {
			r.SubmissionSHA256 = strings.Repeat("a", 64)
		}},
		{"incomplete", func(r *problemvalidation.CheckerTrialReport) { r.Complete = false }},
		{"stop not confirmed", func(r *problemvalidation.CheckerTrialReport) { r.StopConfirmed = false }},
		{"completed wrong answer", func(r *problemvalidation.CheckerTrialReport) { r.Verdict = "WA" }},
		{"different slot", func(r *problemvalidation.CheckerTrialReport) { r.Slot = "other" }},
		{"different candidate", func(r *problemvalidation.CheckerTrialReport) { r.CandidateSHA256 = strings.Repeat("1", 64) }},
		{"different plan", func(r *problemvalidation.CheckerTrialReport) { r.FormalPlanSHA = strings.Repeat("1", 64) }},
		{"forged runtime digest", func(r *problemvalidation.CheckerTrialReport) { r.RuntimeSHA256 = strings.Repeat("1", 64) }},
		{"another coherent runtime", func(r *problemvalidation.CheckerTrialReport) {
			r.RuntimeProfile.Runtime.Run.CPU++
			r.RuntimeSHA256 = r.RuntimeProfile.Digest()
		}},
		{"execution diagnostic", func(r *problemvalidation.CheckerTrialReport) { r.Diagnostic = &problemvalidation.ExecutionDiagnostic{} }},
		{"stop diagnostic", func(r *problemvalidation.CheckerTrialReport) { r.StopDiagnostic = &problemvalidation.StopDiagnostic{} }},
		{"compile usage absent", func(r *problemvalidation.CheckerTrialReport) { r.CompileUsage = nil }},
		{"compile usage exceeds profile", func(r *problemvalidation.CheckerTrialReport) { r.CompileUsage.CPU++ }},
		{"wrong order", func(r *problemvalidation.CheckerTrialReport) { r.Points[0], r.Points[1] = r.Points[1], r.Points[0] }},
		{"short result", func(r *problemvalidation.CheckerTrialReport) { r.Points = r.Points[:1] }},
		{"point not run", func(r *problemvalidation.CheckerTrialReport) { r.Points[1].Verdict = "NOT_RUN" }},
		{"negative point usage", func(r *problemvalidation.CheckerTrialReport) { r.Points[1].Usage.CPU = -1 }},
		{"point usage absent", func(r *problemvalidation.CheckerTrialReport) { r.Points[1].Usage = nil }},
		{"point usage exceeds profile", func(r *problemvalidation.CheckerTrialReport) { r.Points[1].Usage.Memory++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var report problemvalidation.CheckerTrialReport
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			tc.change(&report)
			data := marshal(report)
			reject(t, data, digest(data))
		})
	}
}

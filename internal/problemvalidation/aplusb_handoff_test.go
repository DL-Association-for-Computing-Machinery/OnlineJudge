package problemvalidation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
)

func TestAPlusBHandoffReportCannotBorrowAnotherRun(t *testing.T) {
	target := APlusBHandoffTarget{"test-receiver", strings.Repeat("a", 40), reviewedAPlusBRuntime, "oj-reference-sandbox"}
	h, err := handoffFor(target)
	if err != nil {
		t.Fatal(err)
	}
	wrongTarget := target
	wrongTarget.Runtime = strings.Repeat("b", 64)
	if _, err := handoffFor(wrongTarget); err == nil {
		t.Fatal("packing for an unsupported runtime must fail")
	}
	p, _, err := checkerTrialProfile(aplusbSourceID)
	if err != nil {
		t.Fatal(err)
	}
	p.Text = ReviewedAPlusBLimits()
	pin := reviewedAPlusBReports[1]
	points := []materials.APlusBPoint{{ID: "first"}, {ID: "second"}}
	base := CheckerTrialReport{Schema: "oj.aplusb-isolated-judge-report.v1", CandidateSHA256: h.Candidate,
		FormalPlanSHA: h.Plan, RuntimeProfile: p, RuntimeSHA256: target.Runtime, Verdict: "WA",
		JudgeManifestSHA256: h.Judge, SubmissionSHA256: pin.submission, Slot: target.Slot,
		Complete: true, StopConfirmed: true, CompileUsage: &execution.Usage{},
		Points: []Point{{ID: "first", Verdict: "WA", Usage: &execution.Usage{}}, {ID: "second", Verdict: "NOT_RUN"}}}
	check := func(r CheckerTrialReport) bool {
		t.Helper()
		raw, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, '\n')
		fresh := pin
		fresh.report = handoffDigest(raw)
		return matchesHandoffReport(raw, fresh, h, points)
	}
	if !check(base) {
		t.Fatal("synthetic matching report refused")
	}
	for label, edit := range map[string]func(*CheckerTrialReport){
		"other profile": func(r *CheckerTrialReport) {
			r.RuntimeProfile.Runtime.KernelRelease = "other"
			r.RuntimeSHA256 = r.RuntimeProfile.Digest()
		},
		"other slot":        func(r *CheckerTrialReport) { r.Slot = "other" },
		"other input":       func(r *CheckerTrialReport) { r.JudgeManifestSHA256 = "" },
		"other source":      func(r *CheckerTrialReport) { r.SubmissionSHA256 = reviewedAPlusBReports[0].submission },
		"no stop":           func(r *CheckerTrialReport) { r.StopConfirmed = false },
		"wrong point order": func(r *CheckerTrialReport) { r.Points[1].ID = "first" },
		"ran after failure": func(r *CheckerTrialReport) { r.Points[1].Usage = &execution.Usage{} },
	} {
		t.Run(label, func(t *testing.T) {
			modified := base
			modified.Points = append([]Point(nil), base.Points...)
			edit(&modified)
			if check(modified) {
				t.Fatal("matching byte digest cannot waive the target or report constraints")
			}
		})
	}
}

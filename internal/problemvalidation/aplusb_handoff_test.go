package problemvalidation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
)

func TestAPlusBHandoffSelectsOnlyMatchingRuntimeAndSlot(t *testing.T) {
	desktop := APlusBHandoffTarget{"local-rehearsal", "4184ed4c01297704f702b349c1f4b2e7cb7f4a35", reviewedAPlusBRuntime, "oj-reference-sandbox"}
	ubuntu := APlusBHandoffTarget{desktop.Node, desktop.Revision, ubuntuHandoffRuntime, "oj-aplusb-ubuntu-sandbox"}
	h, err := handoffFor(desktop)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(h)
	if err != nil || handoffDigest(raw) != "ef36b6a9d5b06369448910972fa32acc6a25b32b839bd77e384540bb85484d85" {
		t.Fatal("existing Desktop handoff changed")
	}
	h, err = handoffFor(ubuntu)
	if err != nil || h.Judge == reviewedAPlusBManifest || h.Transport == transportPin {
		t.Fatal("Ubuntu handoff did not select its own material identity")
	}
	for _, target := range []APlusBHandoffTarget{
		{desktop.Node, desktop.Revision, desktop.Runtime, ubuntu.Slot},
		{ubuntu.Node, ubuntu.Revision, ubuntu.Runtime, desktop.Slot},
	} {
		if _, err := handoffFor(target); err == nil {
			t.Fatal("cross-runtime slot accepted")
		}
	}
}

func TestAPlusBUbuntuHandoffRequiresGuestReport(t *testing.T) {
	target := APlusBHandoffTarget{"guest", strings.Repeat("a", 40), ubuntuHandoffRuntime, "oj-aplusb-ubuntu-sandbox"}
	h, err := handoffFor(target)
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := checkerTrialProfile(aplusbSourceID)
	if err != nil {
		t.Fatal(err)
	}
	desktop := p
	config, err := experiment.ResolveReferenceTier(experiment.ReferenceAPlusBUbuntu)
	if err != nil {
		t.Fatal(err)
	}
	p.ID = "p4-checker-candidate-aplusb-ubuntu-reference-v1"
	p.Runtime, p.Files, p.Text = config.Runtime, config.Files, ReviewedAPlusBLimits()
	pin := reviewedAPlusBReports[0]
	points := []materials.APlusBPoint{{ID: "first"}}
	report := CheckerTrialReport{Schema: aplusbJudgeReportSchema, CandidateSHA256: h.Candidate,
		FormalPlanSHA: h.Plan, RuntimeProfile: p, RuntimeSHA256: p.Digest(), Verdict: "AC",
		JudgeManifestSHA256: h.Judge, SubmissionSHA256: pin.submission, Slot: target.Slot,
		Complete: true, StopConfirmed: true, CompileUsage: &execution.Usage{},
		Points: []Point{{ID: "first", Verdict: "AC", Usage: &execution.Usage{}}}}
	check := func(r CheckerTrialReport) bool {
		t.Helper()
		raw, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, '\n')
		pin.report = handoffDigest(raw)
		return matchesHandoffReport(raw, pin, h, points)
	}
	if !check(report) {
		t.Fatal("report for the pinned Ubuntu target was refused")
	}
	report.RuntimeProfile = desktop
	report.RuntimeSHA256 = desktop.Digest()
	if check(report) {
		t.Fatal("Desktop report accepted for the Ubuntu target")
	}
}

func TestAPlusBTrialCandidateHandoffCannotBecomeAReportReceipt(t *testing.T) {
	target := APlusBHandoffTarget{"fixture-trial", strings.Repeat("c", 40), trialAMD64HandoffRuntime, trialAMD64HandoffSlot}
	h, err := handoffFor(target)
	if err != nil || h.Schema != candidateHandoffSchema || h.Judge == ubuntuHandoffJudge || h.Transport == ubuntuHandoffTransport {
		t.Fatalf("missing separate amd64 candidate binding: %v", err)
	}
	raw, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	pin := handoffDigest(raw)
	receipt, err := aplusbReceiptFor(APlusBHandoffReadback{Scope: handoffReadbackScope, Handoff: pin,
		Target: target, Reports: [3]string{strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)}, Points: 17})
	if err == nil || receipt != (APlusBHandoffReceipt{}) {
		t.Fatal("an offline candidate produced a verified report receipt")
	}
	for _, pair := range []struct{ runtime, slot string }{
		{ubuntuHandoffRuntime, target.Slot}, {target.Runtime, "oj-aplusb-ubuntu-sandbox"},
		{reviewedAPlusBRuntime, target.Slot}, {target.Runtime, "oj-reference-sandbox"},
	} {
		wrong := target
		wrong.Runtime, wrong.Slot = pair.runtime, pair.slot
		if _, err := handoffFor(wrong); err == nil {
			t.Fatal("an existing arm64 slot or runtime matched the amd64 candidate")
		}
	}
}

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

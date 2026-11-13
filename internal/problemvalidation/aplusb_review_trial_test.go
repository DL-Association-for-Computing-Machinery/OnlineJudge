package problemvalidation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

func TestAPlusBTrialRequiresExactPointAndStopEvidence(t *testing.T) {
	p, _, err := checkerTrialProfile(aplusbSourceID)
	if err != nil {
		t.Fatal(err)
	}
	p.Text = ReviewedAPlusBLimits()
	trial := CheckerTrial{profile: p, candidateSHA: ReviewedAPlusBCandidate}
	trial.plan.Cases = []problem.ReferenceCase{{ID: "one"}, {ID: "two"}}
	const source = "2017302f68eaa906e16b9a39a5de26bf71d07aef0880dafdd3dc9110c36121cd"
	verify := func(report CheckerTrialReport) bool {
		t.Helper()
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, '\n')
		sum := sha256.Sum256(raw)
		return trial.verifyAPlusBTrial(raw, aplusbTrialPin{"WA", hex.EncodeToString(sum[:]), source})
	}
	base := CheckerTrialReport{Schema: "oj.aplusb-isolated-judge-report.v1", CandidateSHA256: trial.candidateSHA,
		FormalPlanSHA: trial.plan.Digest(), RuntimeProfile: p, RuntimeSHA256: p.Digest(),
		JudgeManifestSHA256: trial.aplusbManifest().Digest(), SubmissionSHA256: source, Verdict: "WA",
		Slot: "oj-reference-sandbox", Complete: true, StopConfirmed: true,
		CompileUsage: &execution.Usage{}, Points: []Point{
			{ID: "one", Verdict: "WA", Usage: &execution.Usage{}},
			{ID: "two", Verdict: "NOT_RUN"},
		}}
	if !verify(base) {
		t.Fatal("known synthetic evidence shape was rejected")
	}
	for name, change := range map[string]func(*CheckerTrialReport){
		"unstopped":               func(r *CheckerTrialReport) { r.StopConfirmed = false },
		"other submission":        func(r *CheckerTrialReport) { r.SubmissionSHA256 = "" },
		"other schema":            func(r *CheckerTrialReport) { r.Schema = "oj.checker-trial.v1" },
		"swapped point":           func(r *CheckerTrialReport) { r.Points[0], r.Points[1] = r.Points[1], r.Points[0] },
		"ran after first failure": func(r *CheckerTrialReport) { r.Points[1].Verdict = "AC" },
		"unknown usage":           func(r *CheckerTrialReport) { r.Points[0].Usage = nil },
		"usage after stop":        func(r *CheckerTrialReport) { r.Points[1].Usage = &execution.Usage{} },
	} {
		t.Run(name, func(t *testing.T) {
			modified := base
			modified.Points = append([]Point(nil), base.Points...)
			change(&modified)
			if verify(modified) {
				t.Fatal("invalid evidence accepted even with a matching byte digest")
			}
		})
	}
}

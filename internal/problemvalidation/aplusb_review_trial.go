package problemvalidation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

const (
	ReviewedAPlusBPlan      = "fe4e1968ae66dbfbe23afb96c03ed62dba8957d06e3b88602ebd653faf1c80f5"
	ReviewedAPlusBCandidate = "84a8b9d0e0a142d5efbe3bc1c410e449856d7fbc2abc598dbfb3178a377c673e"
	reviewedAPlusBManifest  = "a79d761e04d33da6714c987ab0512e3deec4a28531c839f00c95347442437f91"
	reviewedAPlusBRuntime   = "1dacc3c77e44446d598490dd0d808ff32e1bcd1114e715af521406fcc7804ef6"
)

// ReviewedAPlusBLimits returns the explicit limits used in the archived trial.
func ReviewedAPlusBLimits() judging.TextLimits {
	return judging.TextLimits{Bytes: 262144, TokenBytes: 12}
}

type aplusbTrialPin struct{ verdict, report, submission string }

var reviewedAPlusBReports = [...]aplusbTrialPin{
	{"AC", "73fdad6330390c20704401af452dad39707bfea533bc47a73a1cfbc9f3aeab1c", "2017302f68eaa906e16b9a39a5de26bf71d07aef0880dafdd3dc9110c36121cd"},
	{"WA", "f991170e7f456abca1f33956d8d2c56478e925e800706154ec0f405ff4c84502", "0408e9f32a8dcb2f537fcdd3afde7f4a8f6c815efc243630cd194c9598afecd4"},
	{"OLE", "33b231280ac4bceb1ca31e90cae025ad575c4c87cd75609fc8cb03bf05600dc7", "574cd58f3cdbef54c7a8984d542444bbb48ceb5e83b3a558f5facf327d769949"},
}

// LocalCheckerEvidence identifies an archived Desktop experiment. It is not an
// admission decision and contains no individual point identifiers or source.
type LocalCheckerEvidence struct {
	Scope     string `json:"scope"`
	Candidate string `json:"candidate_sha256"`
	Manifest  string `json:"judge_manifest_sha256"`
	Runtime   string `json:"runtime_profile_sha256"`
	Points    int    `json:"points"`
	ACReport  string `json:"ac_report_sha256"`
	WAReport  string `json:"wa_report_sha256"`
	OLEReport string `json:"ole_report_sha256"`
}

func reviewedAPlusBSummary() LocalCheckerEvidence {
	return LocalCheckerEvidence{Scope: "isolated-desktop", Candidate: ReviewedAPlusBCandidate, Manifest: reviewedAPlusBManifest,
		Runtime: reviewedAPlusBRuntime, Points: 17, ACReport: reviewedAPlusBReports[0].report,
		WAReport: reviewedAPlusBReports[1].report, OLEReport: reviewedAPlusBReports[2].report}
}

// APlusBLocalCheckerEvidence matches the exact reviewed import and formal plan.
func APlusBLocalCheckerEvidence(id problem.ImportIdentity, plan, profile string) (LocalCheckerEvidence, bool) {
	if !matchesReviewedApPlusBPlan(id, plan, profile) {
		return LocalCheckerEvidence{}, false
	}
	return reviewedAPlusBSummary(), true
}

// VerifyReviewedAPlusBTrials checks the pinned AC, WA and OLE archive, in that
// order, against the independently rederived formal plan and judge manifest.
func (t *CheckerTrial) VerifyReviewedAPlusBTrials(ac, wa, ole []byte) (LocalCheckerEvidence, error) {
	if t == nil || t.plan.SourceID != aplusbSourceID || t.plan.Digest() != ReviewedAPlusBPlan ||
		t.candidateSHA != ReviewedAPlusBCandidate || t.profile.Digest() != reviewedAPlusBRuntime ||
		t.aplusbManifest().Digest() != reviewedAPlusBManifest || len(t.plan.Cases) != 17 {
		return LocalCheckerEvidence{}, ErrCheckerTrialEvidence
	}
	for i, raw := range [3][]byte{ac, wa, ole} {
		if !t.verifyAPlusBTrial(raw, reviewedAPlusBReports[i]) {
			return LocalCheckerEvidence{}, ErrCheckerTrialEvidence
		}
	}
	return reviewedAPlusBSummary(), nil
}

func (t *CheckerTrial) verifyAPlusBTrial(raw []byte, pin aplusbTrialPin) bool {
	if len(raw) == 0 || len(raw) > MaxCheckerTrialReportBytes {
		return false
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != pin.report {
		return false
	}
	var report CheckerTrialReport
	if json.Unmarshal(raw, &report) != nil {
		return false
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil || !bytes.Equal(raw, append(encoded, '\n')) ||
		report.Schema != "oj.aplusb-isolated-judge-report.v1" || !report.Complete || !report.StopConfirmed ||
		report.Diagnostic != nil || report.StopDiagnostic != nil || report.Slot != "oj-reference-sandbox" ||
		report.CandidateSHA256 != t.candidateSHA || report.FormalPlanSHA != t.plan.Digest() ||
		report.RuntimeSHA256 != t.profile.Digest() || report.RuntimeProfile.Digest() != report.RuntimeSHA256 ||
		report.JudgeManifestSHA256 != t.aplusbManifest().Digest() || report.SubmissionSHA256 != pin.submission ||
		report.Verdict != pin.verdict || !recordedUsage(report.CompileUsage, t.profile.Runtime.Compile) ||
		len(report.Points) != len(t.plan.Cases) {
		return false
	}
	for i, point := range report.Points {
		want := pin.verdict
		if i > 0 && want != "AC" {
			want = "NOT_RUN"
		}
		if point.ID != t.plan.Cases[i].ID || point.Verdict != want {
			return false
		}
		if want == "NOT_RUN" {
			if point.Usage != nil {
				return false
			}
		} else if !recordedUsage(point.Usage, t.profile.Runtime.Run) {
			return false
		}
	}
	return true
}

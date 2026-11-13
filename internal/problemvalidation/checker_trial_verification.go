package problemvalidation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

// MaxCheckerTrialReportBytes bounds a report including its fixed profile and
// at most the admitted number of formal points.
const MaxCheckerTrialReportBytes = 64 << 10

// ErrCheckerTrialEvidence means a stored report does not verify as a complete
// passing run of the rederived trial.
var ErrCheckerTrialEvidence = errors.New("checker trial report is not verified passing evidence")

// CheckerTrialVerification describes a matching private maintenance report.
// It is neither a signature nor an admission or publication decision.
type CheckerTrialVerification struct {
	Schema          string `json:"schema"`
	ReportSHA256    string `json:"report_sha256"`
	CandidateSHA256 string `json:"candidate_sha256"`
	FormalPlanSHA   string `json:"formal_plan_sha256"`
	RuntimeSHA256   string `json:"runtime_profile_sha256"`
	Points          int    `json:"points"`
}

// VerifyPassingReport checks the exact serialized Worker report against this
// preflighted trial. The caller supplies the independently trusted byte digest
// and expected fixed slot, and verifies publication of the completed file.
func (t *CheckerTrial) VerifyPassingReport(raw []byte, expectedSHA256, expectedSlot string) (CheckerTrialVerification, error) {
	var zero CheckerTrialVerification
	if t == nil || t.candidateSHA == "" || expectedSlot == "" || len(raw) == 0 || len(raw) > MaxCheckerTrialReportBytes {
		return zero, ErrCheckerTrialEvidence
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != expectedSHA256 {
		return zero, ErrCheckerTrialEvidence
	}
	var report CheckerTrialReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return zero, ErrCheckerTrialEvidence
	}
	// Reproduce the exact writer format to reject ambiguous or extra fields,
	// including duplicate keys accepted by the standard JSON decoder.
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil || !bytes.Equal(raw, append(encoded, '\n')) {
		return zero, ErrCheckerTrialEvidence
	}
	profile := t.profile.Digest()
	if report.Schema != "oj.checker-trial.v1" || report.JudgeManifestSHA256 != "" || report.SubmissionSHA256 != "" ||
		!report.Complete || !report.StopConfirmed || report.Verdict != "AC" ||
		report.Diagnostic != nil || report.StopDiagnostic != nil || report.Slot != expectedSlot ||
		report.CandidateSHA256 != t.candidateSHA || report.FormalPlanSHA != t.plan.Digest() ||
		report.RuntimeSHA256 != profile || report.RuntimeProfile.Digest() != profile ||
		!recordedUsage(report.CompileUsage, t.profile.Runtime.Compile) || len(report.Points) != len(t.plan.Cases) {
		return zero, ErrCheckerTrialEvidence
	}
	for i, point := range report.Points {
		if point.ID != t.plan.Cases[i].ID || point.Verdict != "AC" || !recordedUsage(point.Usage, t.profile.Runtime.Run) {
			return zero, ErrCheckerTrialEvidence
		}
	}
	return CheckerTrialVerification{
		Schema: "oj.checker-trial-verification.v1", ReportSHA256: expectedSHA256, CandidateSHA256: t.candidateSHA,
		FormalPlanSHA: report.FormalPlanSHA, RuntimeSHA256: profile, Points: len(report.Points),
	}, nil
}

func recordedUsage(u *execution.Usage, l experiment.Limits) bool {
	return u != nil && u.CPU >= 0 && u.CPU <= l.CPU && u.Memory >= 0 && u.Memory <= l.Memory
}

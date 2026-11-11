package problemreference

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

// MaxWorkerEvidenceBytes bounds the compact reference evidence, excluding the
// control plane's separately stored canonical plan and profile.
const MaxWorkerEvidenceBytes = 16 << 10

const workerEvidenceSchema = "oj.reference-worker-evidence.v1"

// WorkerReportInput binds frozen evidence to a particular execution reservation.
type WorkerReportInput struct {
	MaterialInput
	Evidence []byte `json:"evidence"`
}

// WorkerEvidence describes a determinate result from the dedicated trusted
// node, with stop and cleanup attested by its slot owner. It contains no
// materials, diagnostics, or permission to publish.
type WorkerEvidence struct {
	Schema        string                    `json:"schema"`
	ProfileSHA256 string                    `json:"profile_sha256"`
	Verdict       string                    `json:"verdict"`
	CompileUsage  *execution.Usage          `json:"compile_usage"`
	Points        []problemvalidation.Point `json:"points"`
	StopConfirmed bool                      `json:"stop_confirmed"`
	Slot          string                    `json:"slot"`
	ContainerID   string                    `json:"container_id"`
	StartedAt     string                    `json:"started_at"`
	Comparison    *WorkerComparison         `json:"comparison,omitempty"`
}

// WorkerComparison is the determinate outcome of the separately frozen plan,
// attested under the same outer stop and container identity.
type WorkerComparison struct {
	PlanSHA256     string                                       `json:"plan_sha256"`
	Status         string                                       `json:"status"`
	Matched        int                                          `json:"matched"`
	Counterexample *problemvalidation.ConvolutionCounterexample `json:"counterexample,omitempty"`
}

// DecodeEvidence checks syntax and canonical bytes. In particular unknown,
// missing, and duplicate evidence fields cannot silently change the meaning.
func (i WorkerReportInput) DecodeEvidence() (WorkerEvidence, error) {
	var e WorkerEvidence
	if !i.Valid() || len(i.Evidence) == 0 || len(i.Evidence) > MaxWorkerEvidenceBytes || json.Unmarshal(i.Evidence, &e) != nil {
		return WorkerEvidence{}, ErrInvalid
	}
	raw, err := json.Marshal(e)
	if err != nil || !bytes.Equal(raw, i.Evidence) || !e.validShape() {
		return WorkerEvidence{}, ErrInvalid
	}
	return e, nil
}

func (e WorkerEvidence) validShape() bool {
	started, err := time.Parse(time.RFC3339Nano, e.StartedAt)
	return e.Schema == workerEvidenceSchema && e.StopConfirmed && problem.ValidImportRevision(e.ProfileSHA256) &&
		problem.ValidImportRevision(e.ContainerID) && len(e.Slot) > 0 && len(e.Slot) <= 128 && err == nil && !started.IsZero() &&
		len(e.Points) > 0 && len(e.Points) <= 64 && validUsage(e.CompileUsage) && (e.Verdict == "AC" || e.Verdict == "CE" || failedPoint(e.Verdict)) &&
		(e.Comparison == nil || e.Verdict == "AC" && problem.ValidImportRevision(e.Comparison.PlanSHA256))
}

// ValidFor compares the reported order and outcome against control-side
// immutable evidence. It deliberately excludes infrastructure errors (SE).
func (e WorkerEvidence) ValidFor(spec Specification, expected ExpectedPlan) bool {
	var plan problem.ReferencePlan
	var profile problemvalidation.Profile
	if !e.validShape() || !spec.Valid() || e.ProfileSHA256 != spec.ProfileSHA256 || json.Unmarshal(spec.ProfileJSON, &profile) != nil ||
		!expected.ValidFor(spec) || json.Unmarshal(expected.Encoding, &plan) != nil || plan.ValidateMaterials() != nil || len(e.Points) != len(plan.Cases) {
		return false
	}
	if e.Verdict != "CE" && (e.CompileUsage.CPU > profile.Runtime.Compile.CPU || e.CompileUsage.Memory > profile.Runtime.Compile.Memory) {
		return false
	}
	_, tier, err := problemvalidation.ResolveSource(plan.SourceID)
	if err != nil {
		return false
	}
	slot := (workerhost.M0Slot{}).ContainerName()
	if tier != 0 {
		target, err := workerhost.ReferenceSlotForTier(tier)
		if err != nil {
			return false
		}
		slot = target.ContainerName()
	}
	if e.Slot != slot {
		return false
	}
	ended := e.Verdict == "CE"
	for n, p := range e.Points {
		if p.ID != plan.Cases[n].ID {
			return false
		}
		if ended {
			if p.Verdict != "NOT_RUN" || p.Usage != nil {
				return false
			}
			continue
		}
		if !validUsage(p.Usage) {
			return false
		}
		// WA is derived from an accepted sandbox run whose output did not match.
		if (p.Verdict == "AC" || p.Verdict == "WA") && (p.Usage.CPU > profile.Runtime.Run.CPU || p.Usage.Memory > profile.Runtime.Run.Memory) {
			return false
		}
		if p.Verdict != "AC" {
			if p.Verdict != e.Verdict || !failedPoint(p.Verdict) {
				return false
			}
			ended = true
		}
	}
	return ended == (e.Verdict != "AC")
}

func validUsage(u *execution.Usage) bool { return u != nil && u.CPU >= 0 && u.Memory >= 0 }

func failedPoint(verdict string) bool {
	switch verdict {
	case "WA", "RE", "TLE", "MLE", "OLE":
		return true
	default:
		return false
	}
}

// FreezeWorkerReport serializes an owner's finalized determinate report once.
// The owner must finalize only after verifying stop and cleaning the slot.
func FreezeWorkerReport(claim WorkerClaim, report problemvalidation.Report) (WorkerReportInput, error) {
	return freezeWorkerReport(claim, report, nil)
}

// FreezeWorkerReportWithComparison attests the formal and comparison results
// together, after the shared host has stopped and slot cleanup has succeeded.
func FreezeWorkerReportWithComparison(claim WorkerClaim, report problemvalidation.Report, plan problemvalidation.ConvolutionPlan,
	comparison problemvalidation.ConvolutionReport,
) (WorkerReportInput, error) {
	if report.Verdict != "AC" || comparison.StopDiagnostic != nil || !comparison.StopConfirmed ||
		comparison.ContainerID != report.ContainerID || comparison.StartedAt != report.StartedAt ||
		comparison.Schema != "oj.convolution-differential.v1" || comparison.Publication != "blocked" ||
		comparison.Draft == nil || *comparison.Draft != plan.Draft || comparison.SourceID != plan.SourceID ||
		comparison.SourceRevision != plan.SourceRevision || comparison.SourceLock != plan.SourceLock ||
		comparison.Oracle != plan.Oracle || comparison.Candidate != plan.Candidate || comparison.Corpus != plan.Corpus ||
		comparison.Seed != plan.Seed || comparison.CorpusSHA256 != plan.CorpusSHA256 || comparison.ProfileSHA256 != plan.ProfileSHA256 ||
		comparison.Cases != plan.Cases || comparison.OracleParsed != plan.Cases ||
		!plan.ValidOutcome(comparison.Status, comparison.CandidateEqual, comparison.Counterexample) {
		return WorkerReportInput{}, ErrInvalid
	}
	if _, err := plan.MaterialObjects(claim.PlanSHA256); err != nil {
		return WorkerReportInput{}, ErrInvalid
	}
	return freezeWorkerReport(claim, report, &WorkerComparison{
		PlanSHA256: plan.Digest(), Status: comparison.Status, Matched: comparison.CandidateEqual, Counterexample: comparison.Counterexample,
	})
}

// ValidComparison requires comparison evidence for first formal AC exactly
// when a current comparison plan exists. Historical replay is separate.
func (e WorkerEvidence) ValidComparison(expected ExpectedPlan) bool {
	if expected.Differential == nil || e.Verdict != "AC" {
		return e.Comparison == nil
	}
	plan, err := expected.Differential.CurrentPlan(expected.SHA256)
	return err == nil && e.Comparison != nil && e.Comparison.PlanSHA256 == expected.Differential.SHA256 &&
		plan.ValidOutcome(e.Comparison.Status, e.Comparison.Matched, e.Comparison.Counterexample)
}

func freezeWorkerReport(claim WorkerClaim, report problemvalidation.Report, comparison *WorkerComparison) (WorkerReportInput, error) {
	input := WorkerReportInput{MaterialInput: MaterialInput{ClaimInput: ClaimInput{RequestID: claim.RequestID, SourceID: claim.SourceID, PlanSHA256: claim.PlanSHA256}, Token: claim.Token}}
	status := "reference-validation-failed"
	if report.Verdict == "AC" {
		status = "reference-validation-passed"
	}
	if !claim.ValidFor(input.ClaimInput) || report.Plan.SourceID != claim.SourceID || report.PlanSHA256 != claim.PlanSHA256 || report.Plan.Digest() != claim.PlanSHA256 ||
		report.Profile.Digest() != report.ProfileSHA256 || report.StopDiagnostic != nil || report.ExecutionDiagnostic != nil || report.Publication != "blocked" ||
		report.Schema != "oj.reference-validation.v1" || report.Status != status {
		return WorkerReportInput{}, ErrInvalid
	}
	e := WorkerEvidence{Schema: workerEvidenceSchema, ProfileSHA256: report.ProfileSHA256, Verdict: report.Verdict, CompileUsage: report.CompileUsage,
		Points: report.Points, StopConfirmed: report.StopConfirmed, Slot: report.Slot, ContainerID: report.ContainerID, StartedAt: report.StartedAt,
		Comparison: comparison}
	raw, err := json.Marshal(e)
	if err != nil {
		return WorkerReportInput{}, ErrInvalid
	}
	input.Evidence = raw
	if _, err := input.DecodeEvidence(); err != nil {
		return WorkerReportInput{}, err
	}
	return input, nil
}

// WorkerReporter admits a first result or acknowledges an exact immutable replay.
type WorkerReporter interface {
	ReportReference(context.Context, submission.NodeIdentity, WorkerReportInput) error
}

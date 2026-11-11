package problemreference

import (
	"context"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ReviewPolicyVersion identifies the current requirements for ordinary approval.
const ReviewPolicyVersion = "oj.review-approval.v4"

// ReviewEligibilitySnapshot is the trusted read of an assigned item's verified
// historical evidence and whether the same material has already been returned.
type ReviewEligibilitySnapshot struct {
	Evidence          ReviewEvidence
	Returned          bool
	SolutionConfirmed bool
}

// ReviewEligibility explains what blocks approval at the time of the read.
// It is not a receipt or proof that a later decision is permitted.
type ReviewEligibility struct {
	Evidence               ReviewEvidence                                    `json:"evidence"`
	PolicyVersion          string                                            `json:"policy_version"`
	Eligible               bool                                              `json:"eligible"`
	Blockers               []string                                          `json:"blockers"`
	InputValidator         *problemvalidation.InputValidatorEvidence         `json:"input_validator_evidence,omitempty"`
	IndependentCorrectness *problemvalidation.IndependentCorrectnessEvidence `json:"independent_correctness_evidence,omitempty"`
	WrongSolution          *problemvalidation.WrongSolutionEvidence          `json:"wrong_solution_evidence,omitempty"`
}

func assessReviewEligibility(s ReviewEligibilitySnapshot) ReviewEligibility {
	e := s.Evidence
	var blockers []string
	if s.Returned {
		blockers = append(blockers, "materials_returned")
	}
	if e.FormalVerdict != "AC" {
		blockers = append(blockers, "formal_failed")
	}
	profile, _, err := problemvalidation.ResolveSource(e.SourceID)
	policyCurrent := err == nil && e.SourceLockSHA256 == problemvalidation.SourceLockSHA256 && e.ProfileSHA256 == profile.Digest()
	if !policyCurrent {
		// An unknown or older source profile cannot waive a required check.
		blockers = append(blockers, "policy_unavailable")
	}
	id := problem.ImportIdentity{SourceID: e.SourceID, Version: e.ContentVersion, ImportRevision: e.ImportRevision, SourceLockSHA256: e.SourceLockSHA256}
	var independent *problemvalidation.IndependentCorrectnessEvidence
	switch {
	case e.ComparisonOutcome == "mismatch":
		blockers = append(blockers, "comparison_mismatch")
	case e.SourceID == problemvalidation.ConvolutionSourceID:
		if e.ComparisonOutcome != "passed" {
			blockers = append(blockers, "comparison_missing")
		}
	default:
		if policyCurrent {
			if admitted, ok := problemvalidation.AdmittedIndependentCorrectness(id, e.FormalPlanSHA256); ok {
				independent = &admitted
			}
		}
		if independent == nil {
			blockers = append(blockers, "independent_correctness_missing")
		}
	}
	// The input check is independent of the reference verdict and confirms
	// neither statement quality nor coverage or production readiness.
	blockers = append(blockers, "statement_quality_missing", "test_coverage_missing")
	var validator *problemvalidation.InputValidatorEvidence
	var wrongSolution *problemvalidation.WrongSolutionEvidence
	if policyCurrent {
		if admitted, ok := problemvalidation.AdmittedInputValidator(id, e.FormalPlanSHA256); ok {
			validator = &admitted
		}
		if admitted, ok := problemvalidation.AdmittedWrongSolution(id, e.FormalPlanSHA256); ok {
			wrongSolution = &admitted
		}
	}
	if validator == nil {
		blockers = append(blockers, "input_validator_missing")
	}
	blockers = append(blockers, "checker_admission_missing", "target_runtime_missing")
	if !s.SolutionConfirmed {
		blockers = append(blockers, "solution_confirmation_missing")
	}
	return ReviewEligibility{Evidence: e, PolicyVersion: ReviewPolicyVersion, Eligible: len(blockers) == 0,
		Blockers: blockers, InputValidator: validator, IndependentCorrectness: independent, WrongSolution: wrongSolution}
}

// EligibilityForSubject returns the current policy's blockers for one assignment.
func (s *ReviewAccess) EligibilityForSubject(ctx context.Context, c redisstore.Credential, expected, id string) (ReviewEligibility, error) {
	var zero ReviewEligibility
	actor, err := s.reviewActor(ctx, c, expected, id)
	if err != nil {
		return zero, err
	}
	snapshot, err := s.repo.ReviewEligibilitySnapshot(ctx, actor, id)
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	e := snapshot.Evidence
	if !e.Valid() || e.ReviewID != id || e.Reviewer != expected {
		return zero, ErrStore
	}
	return assessReviewEligibility(snapshot), nil
}

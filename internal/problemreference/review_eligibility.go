package problemreference

import (
	"context"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ReviewPolicyVersion identifies the current requirements for ordinary approval.
const ReviewPolicyVersion = "oj.review-approval.v14"

// LocalReviewPolicyVersion reserves a distinct policy for explicitly scoped
// functional acceptance. Its approvals cannot authorize ordinary publication.
const LocalReviewPolicyVersion = "oj.review-approval.aplusb-local.v1"

// ReviewEligibilitySnapshot is the trusted read of an assigned item's verified
// historical evidence and whether the same material has already been returned.
type ReviewEligibilitySnapshot struct {
	Evidence           ReviewEvidence
	Returned           bool
	SolutionConfirmed  bool
	StatementConfirmed bool
	LocalExecution     problemvalidation.APlusBLocalAdmission
}

// ReviewEligibility explains what blocks approval at the time of the read.
// It is not a receipt or proof that a later decision is permitted.
type ReviewEligibility struct {
	Evidence                    ReviewEvidence                                    `json:"evidence"`
	PolicyVersion               string                                            `json:"policy_version"`
	SnapshotSHA256              string                                            `json:"snapshot_sha256"`
	Eligible                    bool                                              `json:"eligible"`
	Blockers                    []string                                          `json:"blockers"`
	InputValidator              *problemvalidation.InputValidatorEvidence         `json:"input_validator_evidence,omitempty"`
	IndependentCorrectness      *problemvalidation.IndependentCorrectnessEvidence `json:"independent_correctness_evidence,omitempty"`
	WrongSolution               *problemvalidation.WrongSolutionEvidence          `json:"wrong_solution_evidence,omitempty"`
	TestCoverage                *problemvalidation.TestCoverageEvidence           `json:"test_coverage_evidence,omitempty"`
	StatementRendering          *problemvalidation.StatementRenderingEvidence     `json:"statement_rendering_evidence,omitempty"`
	LocalChecker                *problemvalidation.LocalCheckerEvidence           `json:"local_checker_evidence,omitempty"`
	StatementSemanticsConfirmed bool                                              `json:"statement_semantics_confirmed,omitempty"`
	LocalExecutionSHA256        string                                            `json:"local_execution_sha256,omitempty"`
}

func assessReviewEligibility(s ReviewEligibilitySnapshot) ReviewEligibility {
	e := s.Evidence
	blockers := make([]string, 0)
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
	formalOnly := e.ComparisonPlan == "" && e.ComparisonOutcome == ""
	var validator *problemvalidation.InputValidatorEvidence
	var independent *problemvalidation.IndependentCorrectnessEvidence
	if policyCurrent && formalOnly {
		if input, small, ok := problemvalidation.AdmittedApPlusBQuality(id, e.FormalPlanSHA256, e.ProfileSHA256); ok {
			validator, independent = &input, &small
		} else if input, small, ok := problemvalidation.AdmittedUnionFindQuality(id, e.FormalPlanSHA256, e.ProfileSHA256); ok {
			validator, independent = &input, &small
		} else if input, small, ok := problemvalidation.AdmittedZAlgorithmQuality(id, e.FormalPlanSHA256, e.ProfileSHA256); ok {
			validator, independent = &input, &small
		}
	}
	switch {
	case e.ComparisonOutcome == "mismatch":
		blockers = append(blockers, "comparison_mismatch")
	case e.SourceID == problemvalidation.ConvolutionSourceID:
		if e.ComparisonOutcome != "passed" {
			blockers = append(blockers, "comparison_missing")
		}
	default:
		if policyCurrent && independent == nil {
			if admitted, ok := problemvalidation.AdmittedIndependentCorrectness(id, e.FormalPlanSHA256); ok {
				independent = &admitted
			}
		}
		if independent == nil {
			blockers = append(blockers, "independent_correctness_missing")
		}
	}
	var wrongSolution *problemvalidation.WrongSolutionEvidence
	var statementRendering *problemvalidation.StatementRenderingEvidence
	if policyCurrent {
		if validator == nil {
			if admitted, ok := problemvalidation.AdmittedInputValidator(id, e.FormalPlanSHA256); ok {
				validator = &admitted
			}
		}
		if admitted, ok := problemvalidation.AdmittedWrongSolution(id, e.FormalPlanSHA256); ok {
			wrongSolution = &admitted
		}
		if admitted, ok := problemvalidation.AdmittedStatementRendering(id, e.FormalPlanSHA256); ok {
			statementRendering = &admitted
		} else if formalOnly {
			if admitted, ok := problemvalidation.AdmittedRevisedStatementRendering(id, e.FormalPlanSHA256, e.ProfileSHA256); ok {
				statementRendering = &admitted
			}
		}
	}
	semanticsConfirmed := s.StatementConfirmed && !s.Returned
	if !semanticsConfirmed || statementRendering == nil {
		blockers = append(blockers, "statement_quality_missing")
	}
	var coverage *problemvalidation.TestCoverageEvidence
	if policyCurrent && !s.Returned && e.FormalVerdict == "AC" && formalOnly {
		if admitted, ok := problemvalidation.AdmittedTestCoverage(id, e.FormalPlanSHA256, e.ProfileSHA256); ok {
			coverage = &admitted
		}
	}
	if coverage == nil {
		blockers = append(blockers, "test_coverage_missing")
	}
	if validator == nil {
		blockers = append(blockers, "input_validator_missing")
	}
	policyVersion := ReviewPolicyVersion
	var localExecution string
	if policyCurrent && formalOnly && e.FormalVerdict == "AC" && !s.Returned && s.LocalExecution.Match(id, e.FormalPlanSHA256, e.ProfileSHA256) {
		policyVersion = LocalReviewPolicyVersion
		localExecution, _, _ = s.LocalExecution.Binding()
	} else {
		blockers = append(blockers, "checker_admission_missing", "target_runtime_missing")
	}
	var localChecker *problemvalidation.LocalCheckerEvidence
	if policyCurrent && formalOnly && !s.Returned && e.FormalVerdict == "AC" {
		if local, ok := problemvalidation.APlusBLocalCheckerEvidence(id, e.FormalPlanSHA256, e.ProfileSHA256); ok {
			localChecker = &local
		}
	}
	if !s.SolutionConfirmed {
		blockers = append(blockers, "solution_confirmation_missing")
	}
	return ReviewEligibility{Evidence: e, PolicyVersion: policyVersion, Eligible: len(blockers) == 0,
		Blockers: blockers, InputValidator: validator, IndependentCorrectness: independent,
		WrongSolution: wrongSolution, TestCoverage: coverage, StatementRendering: statementRendering, LocalChecker: localChecker,
		StatementSemanticsConfirmed: semanticsConfirmed, LocalExecutionSHA256: localExecution}
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
	return bindReviewEligibility(actor, snapshot)
}

func bindReviewEligibility(actor identity.ProblemActor, snapshot ReviewEligibilitySnapshot) (ReviewEligibility, error) {
	if !actor.Valid() || !snapshot.Evidence.Valid() || snapshot.Evidence.Reviewer != actor.SubjectID {
		return ReviewEligibility{}, ErrInvalid
	}
	status := assessReviewEligibility(snapshot)
	sum, err := reviewEligibilityDigest(actor, snapshot, status)
	if err != nil {
		return ReviewEligibility{}, err
	}
	status.SnapshotSHA256 = sum
	return status, nil
}

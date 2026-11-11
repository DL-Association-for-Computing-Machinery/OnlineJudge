package problemreference

import (
	"context"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ReviewPolicyVersion identifies the current requirements for ordinary approval.
const ReviewPolicyVersion = "oj.review-approval.v1"

// ReviewEligibilitySnapshot is the trusted read of an assigned item's verified
// historical evidence and whether the same material has already been returned.
type ReviewEligibilitySnapshot struct {
	Evidence ReviewEvidence
	Returned bool
}

// ReviewEligibility explains what blocks approval at the time of the read.
// It is not a receipt or proof that a later decision is permitted.
type ReviewEligibility struct {
	Evidence      ReviewEvidence `json:"evidence"`
	PolicyVersion string         `json:"policy_version"`
	Eligible      bool           `json:"eligible"`
	Blockers      []string       `json:"blockers"`
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
	if err != nil || e.SourceLockSHA256 != problemvalidation.SourceLockSHA256 || e.ProfileSHA256 != profile.Digest() {
		// An unknown or older source profile cannot waive a required check.
		blockers = append(blockers, "policy_unavailable")
	}
	switch {
	case e.ComparisonOutcome == "mismatch":
		blockers = append(blockers, "comparison_mismatch")
	case e.SourceID == problemvalidation.ConvolutionSourceID:
		if e.ComparisonOutcome != "passed" {
			blockers = append(blockers, "comparison_missing")
		}
	default:
		// No trusted independent-correctness producer or exemption is
		// registered for the other sources.
		blockers = append(blockers, "independent_correctness_missing")
	}
	// Reference reports do not establish any of these requirements. Keep
	// them blocking until their own revision-bound evidence is connected.
	blockers = append(blockers, "statement_quality_missing", "test_coverage_missing",
		"input_validator_missing", "checker_admission_missing", "target_runtime_missing", "solution_confirmation_missing")
	return ReviewEligibility{Evidence: e, PolicyVersion: ReviewPolicyVersion, Eligible: len(blockers) == 0, Blockers: blockers}
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

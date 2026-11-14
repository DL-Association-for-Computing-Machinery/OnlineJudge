package problemreference

import (
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

func TestCurrentReviewApprovalBindingRequiresFreshReviewAfterChanges(t *testing.T) {
	reviewer, reviewID := strings.Repeat("a", 32), strings.Repeat("b", 32)
	digest, replacement := strings.Repeat("c", 64), strings.Repeat("d", 64)
	saved := ReviewApprovalBinding{ReviewID: reviewID, Reviewer: reviewer, AssignmentRevision: 3, ReviewerRevision: 2,
		ImportRevision: digest, FormalPlanSHA256: digest, EvidenceSHA256: digest, PolicyVersion: ReviewPolicyVersion}
	grant := identity.ProblemReviewerRole{SubjectID: reviewer, Enabled: true, Revision: 2}
	assignment := ReviewAssignment{ReviewID: reviewID, ReviewerSubjectID: reviewer, Revision: 3, ReviewerRevision: 2}
	current := ReviewEvidence{Reviewer: reviewer, ReviewID: reviewID, DraftID: reviewID, ReferenceRequestID: reviewID, AssignmentRevision: 3,
		SourceID: "library-checker:aplusb", FormalVerdict: "AC", ContentVersion: digest, ImportRevision: digest,
		SourceLockSHA256: digest, FormalPlanSHA256: digest, ProfileSHA256: digest, EvidenceSHA256: digest}
	if err := CheckCurrentReviewApprovalBinding(saved, grant, assignment, current); err != nil {
		t.Fatalf("unchanged binding rejected: %v", err)
	}
	for _, tc := range []struct {
		name string
		edit func(*ReviewApprovalBinding, *identity.ProblemReviewerRole, *ReviewAssignment, *ReviewEvidence)
	}{
		{"revoked", func(_ *ReviewApprovalBinding, g *identity.ProblemReviewerRole, _ *ReviewAssignment, _ *ReviewEvidence) {
			g.Enabled = false
		}},
		{"grant revision changed", func(_ *ReviewApprovalBinding, g *identity.ProblemReviewerRole, _ *ReviewAssignment, _ *ReviewEvidence) {
			g.Revision++
		}},
		{"assignment tied to another grant", func(_ *ReviewApprovalBinding, _ *identity.ProblemReviewerRole, a *ReviewAssignment, _ *ReviewEvidence) {
			a.ReviewerRevision++
		}},
		{"assigned to another reviewer", func(_ *ReviewApprovalBinding, _ *identity.ProblemReviewerRole, a *ReviewAssignment, _ *ReviewEvidence) {
			a.ReviewerSubjectID = strings.Repeat("f", 32)
		}},
		{"same reviewer granted again", func(_ *ReviewApprovalBinding, g *identity.ProblemReviewerRole, a *ReviewAssignment, e *ReviewEvidence) {
			g.Revision++
			a.ReviewerRevision++
			a.Revision++
			e.AssignmentRevision++
		}},
		{"withdrawn", func(_ *ReviewApprovalBinding, _ *identity.ProblemReviewerRole, a *ReviewAssignment, _ *ReviewEvidence) {
			a.ReviewerSubjectID = ""
		}},
		{"same reviewer assigned again", func(_ *ReviewApprovalBinding, _ *identity.ProblemReviewerRole, a *ReviewAssignment, e *ReviewEvidence) {
			a.Revision++
			e.AssignmentRevision++
		}},
		{"different review", func(_ *ReviewApprovalBinding, _ *identity.ProblemReviewerRole, a *ReviewAssignment, e *ReviewEvidence) {
			a.ReviewID = strings.Repeat("e", 32)
			e.ReviewID = a.ReviewID
		}},
		{"same content new import", func(_ *ReviewApprovalBinding, _ *identity.ProblemReviewerRole, _ *ReviewAssignment, e *ReviewEvidence) {
			e.ImportRevision = replacement
		}},
		{"new plan", func(_ *ReviewApprovalBinding, _ *identity.ProblemReviewerRole, _ *ReviewAssignment, e *ReviewEvidence) {
			e.FormalPlanSHA256 = replacement
		}},
		{"new evidence", func(_ *ReviewApprovalBinding, _ *identity.ProblemReviewerRole, _ *ReviewAssignment, e *ReviewEvidence) {
			e.EvidenceSHA256 = replacement
		}},
		{"old policy", func(s *ReviewApprovalBinding, _ *identity.ProblemReviewerRole, _ *ReviewAssignment, _ *ReviewEvidence) {
			s.PolicyVersion = "previous-policy"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, g, a, e := saved, grant, assignment, current
			tc.edit(&s, &g, &a, &e)
			if err := CheckCurrentReviewApprovalBinding(s, g, a, e); !errors.Is(err, ErrReviewChanged) {
				t.Fatalf("stale approval should require a fresh review: %v", err)
			}
		})
	}
	if err := CheckCurrentReviewApprovalBinding(ReviewApprovalBinding{}, grant, assignment, current); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing approval accepted: %v", err)
	}
	current.EvidenceSHA256 = "bad"
	if err := CheckCurrentReviewApprovalBinding(saved, grant, assignment, current); !errors.Is(err, ErrInvalid) {
		t.Fatalf("malformed current evidence accepted: %v", err)
	}
}

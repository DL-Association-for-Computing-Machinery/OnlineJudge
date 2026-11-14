package problemreference

import (
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

// ReviewApprovalBinding is the immutable identity and evidence saved with an
// ordinary approval. It carries no authority to publish on its own.
type ReviewApprovalBinding struct {
	ReviewID, Reviewer                               string
	AssignmentRevision, ReviewerRevision             int64
	ImportRevision, FormalPlanSHA256, EvidenceSHA256 string
	PolicyVersion                                    string
}

// CheckCurrentReviewApprovalBinding rejects an approval after a grant,
// assignment, evidence or policy change. The caller must obtain the saved
// approval and all current values from trusted storage under publication locks;
// it must also recheck current eligibility and independent publish authority.
func CheckCurrentReviewApprovalBinding(saved ReviewApprovalBinding, grant identity.ProblemReviewerRole, assignment ReviewAssignment, current ReviewEvidence) error {
	if !identity.ValidID(saved.ReviewID) || !identity.ValidID(saved.Reviewer) ||
		saved.AssignmentRevision < 1 || saved.ReviewerRevision < 1 || saved.PolicyVersion == "" ||
		!problem.ValidImportRevision(saved.ImportRevision) || !problem.ValidImportRevision(saved.FormalPlanSHA256) ||
		!problem.ValidImportRevision(saved.EvidenceSHA256) || !current.Valid() {
		return ErrInvalid
	}
	if !grant.Enabled || grant.SubjectID != saved.Reviewer || grant.Revision != saved.ReviewerRevision ||
		assignment.ReviewID != saved.ReviewID || assignment.ReviewerSubjectID != saved.Reviewer ||
		assignment.Revision != saved.AssignmentRevision || assignment.ReviewerRevision != saved.ReviewerRevision ||
		current.ReviewID != saved.ReviewID || current.Reviewer != saved.Reviewer || current.AssignmentRevision != saved.AssignmentRevision ||
		current.ImportRevision != saved.ImportRevision || current.FormalPlanSHA256 != saved.FormalPlanSHA256 ||
		current.EvidenceSHA256 != saved.EvidenceSHA256 || saved.PolicyVersion != ReviewPolicyVersion {
		return ErrReviewChanged
	}
	return nil
}

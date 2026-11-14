package problemreference

import (
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

// ErrReviewAlreadyApproved means unchanged material already has an approval.
var ErrReviewAlreadyApproved = errors.New("unchanged review materials already approved")

// ApprovalIntent binds one decision to the current assignment and snapshot.
type ApprovalIntent struct {
	RequestKey         string
	AssignmentRevision int64
	SnapshotSHA256     string
	Reason             string
}

// Valid checks the bounded decision input without authorizing it.
func (i ApprovalIntent) Valid() bool {
	return problem.ValidKey(i.RequestKey) && i.AssignmentRevision > 0 &&
		problem.ValidImportRevision(i.SnapshotSHA256) && validReturnReason(i.Reason)
}

// ReviewApprovalReceipt describes a committed audit row, not publish authority.
type ReviewApprovalReceipt struct {
	Binding ReviewApprovalBinding
	ApprovalIntent
	ApprovedAt time.Time
}

// Valid checks the committed receipt shape. Current eligibility must be rechecked separately.
func (r ReviewApprovalReceipt) Valid() bool {
	b := r.Binding
	return r.ApprovalIntent.Valid() && identity.ValidID(b.ReviewID) && identity.ValidID(b.Reviewer) &&
		b.AssignmentRevision == r.AssignmentRevision && b.ReviewerRevision > 0 && b.PolicyVersion != "" &&
		len(b.PolicyVersion) <= 80 && problem.ValidImportRevision(b.ImportRevision) &&
		problem.ValidImportRevision(b.FormalPlanSHA256) && problem.ValidImportRevision(b.EvidenceSHA256) &&
		!r.ApprovedAt.IsZero()
}

package problemreference

import (
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

// ErrReviewApprovalBlocked means the current policy still has unmet requirements.
var ErrReviewApprovalBlocked = errors.New("current review snapshot does not satisfy approval policy")

// CheckReviewApprovalSnapshot compares the digest the reviewer saw with the
// current, server-side assessment. Callers must load current identity,
// assignment and evidence under the same locks as a subsequent decision write.
// The digest only detects changes; this check does not establish authorization.
func CheckReviewApprovalSnapshot(actor identity.ProblemActor, current ReviewEligibilitySnapshot, expectedSHA256 string) error {
	if !problem.ValidImportRevision(expectedSHA256) {
		return ErrInvalid
	}
	status, err := bindReviewEligibility(actor, current)
	if err != nil {
		return err
	}
	if status.SnapshotSHA256 != expectedSHA256 {
		return ErrReviewChanged
	}
	if !status.Eligible {
		return ErrReviewApprovalBlocked
	}
	return nil
}

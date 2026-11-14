package problemreference

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// reviewEligibilityDigest includes the actual server-side actor and the ordered
// policy result. Assessment and any future recheck must use the same encoding.
// It is a comparison token, not an authorization, signed proof or receipt.
func reviewEligibilityDigest(actor identity.ProblemActor, snapshot ReviewEligibilitySnapshot, status ReviewEligibility) (string, error) {
	status.SnapshotSHA256 = ""
	raw, err := json.Marshal(struct {
		Schema             string
		Reviewer           string
		RecoveryID         string
		AuthGeneration     int64
		Returned           bool
		SolutionConfirmed  bool
		StatementConfirmed bool
		Result             ReviewEligibility
	}{
		Schema: "oj.review-eligibility-snapshot.v1", Reviewer: actor.SubjectID,
		RecoveryID: actor.RecoveryID, AuthGeneration: actor.AuthGeneration,
		Returned: snapshot.Returned, SolutionConfirmed: snapshot.SolutionConfirmed,
		StatementConfirmed: snapshot.StatementConfirmed, Result: status,
	})
	if err != nil {
		return "", ErrStore
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

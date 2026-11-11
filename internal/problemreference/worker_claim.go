package problemreference

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// ErrClaimUnavailable intentionally does not distinguish missing, stale, busy,
// unprepared or cancelled requests at the Worker boundary.
var ErrClaimUnavailable = errors.New("reference claim unavailable")

var referenceNodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// ValidReferenceNodeID checks the configured node identifier syntax.
func ValidReferenceNodeID(id string) bool { return referenceNodePattern.MatchString(id) }

// ClaimInput binds a targeted claim to an operator-selected request and plan.
type ClaimInput struct {
	RequestID  string `json:"request_id"`
	SourceID   string `json:"source_id"`
	PlanSHA256 string `json:"plan_sha256"`
}

// Valid checks only syntax; the controller verifies authorization and equality.
func (i ClaimInput) Valid() bool {
	return identity.ValidID(i.RequestID) && len(i.SourceID) <= 128 && sourceIdentityPattern.MatchString(i.SourceID) &&
		problem.ValidImportRevision(i.PlanSHA256)
}

// WorkerClaim is a short reservation, not a material or execution grant.
type WorkerClaim struct {
	RequestID  string    `json:"request_id"`
	SourceID   string    `json:"source_id"`
	PlanSHA256 string    `json:"plan_sha256"`
	Token      string    `json:"lease_token"`
	Until      time.Time `json:"lease_until"`
}

// ValidFor validates a reply against the original request without comparing clocks.
func (c WorkerClaim) ValidFor(i ClaimInput) bool {
	return c.RequestID == i.RequestID && c.SourceID == i.SourceID && c.PlanSHA256 == i.PlanSHA256 &&
		problem.ValidImportRevision(c.Token) && !c.Until.IsZero()
}

// WorkerClaimer reserves a reference request only after live transactional checks.
type WorkerClaimer interface {
	ClaimReference(context.Context, submission.NodeIdentity, ClaimInput) (WorkerClaim, error)
}

// WorkerRenewer extends only a current, fully bound reservation after rechecking
// live authority. Expiry is not proof that the prior sandbox has stopped.
type WorkerRenewer interface {
	RenewReference(context.Context, submission.NodeIdentity, MaterialInput) (WorkerClaim, error)
}

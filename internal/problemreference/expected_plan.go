package problemreference

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

var (
	// ErrNotQueued means the current request cannot accept a plan.
	ErrNotQueued = errors.New("reference request is not queued")
	// ErrAuthorityChanged means current author authority differs from the request's original snapshot.
	ErrAuthorityChanged = errors.New("reference request authorization changed")
	// ErrPlanConflict means a different expected plan has already been recorded.
	ErrPlanConflict = errors.New("reference request already has a different expected plan")
)

// MaxExpectedPlanBytes bounds the identity-only encoding persisted in schema 10.
const MaxExpectedPlanBytes = 64 << 10

// ExpectedPlan holds the exact ReferencePlan JSON and its digest. It contains
// identities, not private material bytes, an execution grant or a verdict.
type ExpectedPlan struct {
	Encoding []byte
	SHA256   string
}

// ValidFor checks the bounded canonical encoding and its historical import
// identity without re-resolving the currently supported profile.
func (p ExpectedPlan) ValidFor(spec Specification) bool {
	if len(p.Encoding) == 0 || len(p.Encoding) > MaxExpectedPlanBytes || profileDigest(p.Encoding) != p.SHA256 {
		return false
	}
	var plan problem.ReferencePlan
	if json.Unmarshal(p.Encoding, &plan) != nil || len(plan.Cases) == 0 || len(plan.Cases) > 64 {
		return false
	}
	i := spec.Identity
	if plan.SourceID != i.SourceID || plan.Version != i.Version || plan.ImportRevision != i.ImportRevision || plan.SourceLockSHA256 != i.SourceLockSHA256 {
		return false
	}
	raw, err := json.Marshal(plan)
	return err == nil && bytes.Equal(raw, p.Encoding)
}

// PreparedPlan is produced only after the internal service freezes and closes a
// complete snapshot. Persistence never accepts a caller-supplied plan or digest.
type PreparedPlan struct {
	request Request
	plan    ExpectedPlan
}

// Values returns detached inputs for the repository's final checks and write.
func (p PreparedPlan) Values() (Request, ExpectedPlan) {
	r, plan := p.request, p.plan
	r.Specification = r.Specification.Clone()
	plan.Encoding = bytes.Clone(plan.Encoding)
	return r, plan
}

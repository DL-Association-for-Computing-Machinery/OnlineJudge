package problemreference

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"unicode/utf8"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

// MaxProfileBytes bounds frozen configuration JSON, not materials or execution.
const MaxProfileBytes = 16 << 10

var sourceIdentityPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*:[a-z0-9][a-z0-9_.:/-]*$`)

// Specification freezes the trusted import and the exact configuration bytes.
// It is an intent identity, not evidence of material health or successful judging.
type Specification struct {
	Identity      problem.ImportIdentity
	Protocol      string
	ProfileJSON   []byte
	ProfileSHA256 string
}

// Valid checks stored identity shape and byte integrity without re-resolving
// current profiles. Retired configurations remain readable at their old keys.
func (s Specification) Valid() bool {
	i := s.Identity
	if len(i.SourceID) > 128 || !sourceIdentityPattern.MatchString(i.SourceID) ||
		!problem.ValidImportRevision(i.Version) || !problem.ValidImportRevision(i.ImportRevision) ||
		!problem.ValidImportRevision(i.SourceLockSHA256) || s.Protocol != IdentityProtocol ||
		len(s.ProfileJSON) == 0 || len(s.ProfileJSON) > MaxProfileBytes || !utf8.Valid(s.ProfileJSON) ||
		!problem.ValidImportRevision(s.ProfileSHA256) {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(s.ProfileJSON, &object) != nil || len(object) == 0 {
		return false
	}
	return profileDigest(s.ProfileJSON) == s.ProfileSHA256
}

// Clone returns a detached specification, including independently owned bytes.
func (s Specification) Clone() Specification {
	s.ProfileJSON = bytes.Clone(s.ProfileJSON)
	return s
}

func profileDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Catalog is immutable trusted server configuration. It retains metadata only,
// so selection never acquires a Prepared, opens materials, or executes a program.
type Catalog struct {
	sources map[string]Specification
}

// NewCatalog captures zero to ten loaded preparations with one trusted source
// lock. The LoadBundle caller fixes that lock; this does not admit a Runner or
// establish current material health. Empty catalogs allow old-key recovery.
func NewCatalog(sources map[string]*problem.Prepared) (*Catalog, error) {
	return NewCatalogWithLocalPackage(sources, problemvalidation.LocalExecutionAdmission{})
}

// NewCatalogWithLocalPackage opts only the authenticated local import into
// the fixed profile. Ordinary catalogs still use the historical source registry.
func NewCatalogWithLocalPackage(sources map[string]*problem.Prepared, local problemvalidation.LocalExecutionAdmission) (*Catalog, error) {
	if len(sources) > 10 {
		return nil, ErrInvalid
	}
	c := &Catalog{sources: make(map[string]Specification, len(sources))}
	var sourceLock string
	for sourceID, prepared := range sources {
		if prepared == nil {
			return nil, ErrInvalid
		}
		identity, err := prepared.ImportIdentity()
		if err != nil || identity.SourceID != sourceID {
			return nil, ErrInvalid
		}
		profile, _, err := problemvalidation.ResolveReferenceProfile(sourceID, local.PackageReview(identity))
		if err != nil {
			return nil, ErrInvalid
		}
		raw, err := json.Marshal(profile)
		if err != nil {
			return nil, ErrInvalid
		}
		spec := Specification{Identity: identity, Protocol: IdentityProtocol,
			ProfileJSON: raw, ProfileSHA256: profileDigest(raw)}
		if !spec.Valid() || sourceLock != "" && sourceLock != identity.SourceLockSHA256 {
			return nil, ErrInvalid
		}
		sourceLock = identity.SourceLockSHA256
		c.sources[sourceID] = spec
	}
	return c, nil
}

// Select requires the exact source, document version and full import revision
// locked by the repository. Ownership and author permission remain its concern.
func (c *Catalog) Select(draft problem.DraftIdentity) (Specification, error) {
	if c == nil {
		return Specification{}, ErrSourceChanged
	}
	spec, ok := c.sources[draft.SourceID]
	if !ok || spec.Identity.Version != draft.Version || spec.Identity.ImportRevision != draft.ImportRevision {
		return Specification{}, ErrSourceChanged
	}
	return spec.Clone(), nil
}

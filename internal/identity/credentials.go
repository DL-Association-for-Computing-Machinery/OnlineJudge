// Package identity owns account credential state, independently of registration
// identifiers and resource permissions. Its persistence API is trusted internal
// infrastructure, never an unauthenticated account-management interface.
package identity

import (
	"errors"
	"regexp"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

var (
	ErrInvalid  = errors.New("invalid identity credential request")
	ErrNotFound = errors.New("identity credentials not found")
	ErrConflict = errors.New("identity credentials changed or operation not allowed")
	ErrStore    = errors.New("identity storage operation failed; outcome may be unknown")
)

// StoredSaltBytes and StoredDigestBytes bound the version-1 storage envelope.
// They do not select production KDF parameters or an approved security policy.
const (
	StoredSaltBytes   = 64
	StoredDigestBytes = 64
)

var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

// ValidID accepts an opaque 128-bit identifier, not an email or login name.
// Callers generate unique IDs and must never reuse an old subject's ID.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Revision identifies both credential and account-revocation facts.
type Revision struct {
	CredentialVersion int64
	AuthGeneration    int64
}

// Valid rejects absent or invalid CAS preconditions.
func (r Revision) Valid() bool { return r.CredentialVersion > 0 && r.AuthGeneration > 0 }

// State contains no password material. Enabled is an account switch, not proof
// of email activation, training access, roles, or permission to issue a session.
type State struct {
	ID      string
	Enabled bool
	Revision
}

// Snapshot is sensitive internal verification material; never log or send it
// to HTTP clients. A read remains a point-in-time observation.
type Snapshot struct {
	State
	Record password.Record
}

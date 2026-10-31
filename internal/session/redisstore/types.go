// Package redisstore implements the trusted ordinary-session Redis protocol.
// It issues no HTTP cookies and grants no resource or email-verification rights.
package redisstore

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

var (
	ErrInvalid  = errors.New("invalid session protocol input")
	ErrRandom   = errors.New("session credential generation failed")
	ErrStore    = errors.New("session storage failed; outcome may be unknown")
	ErrUnknown  = errors.New("session admission outcome unknown")
	ErrLimit    = errors.New("ordinary session limit reached")
	ErrStale    = errors.New("session expired or revoked")
	ErrNotFound = errors.New("session record absent; revocation unconfirmed")
	ErrConflict = errors.New("session record changed or inconsistent")
)

// Policy bounds ordinary sessions. All stores in a namespace must use the same
// policy. Shorter intervals support controlled environments; never exceed the
// confirmed ordinary limits. This does not support remembered/admin profiles.
type Policy struct{ Idle, Absolute time.Duration }

// OrdinaryPolicy returns the confirmed two-hour idle/twelve-hour absolute limits.
func OrdinaryPolicy() Policy { return Policy{Idle: 2 * time.Hour, Absolute: 12 * time.Hour} }

func (p Policy) valid() bool {
	return p.Idle >= time.Millisecond && p.Absolute > p.Idle && p.Idle <= 2*time.Hour && p.Absolute <= 12*time.Hour &&
		p.Idle%time.Millisecond == 0 && p.Absolute%time.Millisecond == 0
}

var namespacePattern = regexp.MustCompile(`^[a-z0-9_-]{1,48}$`)

// Credential is a sensitive opaque token. Parse browser input at the boundary;
// possession still requires a fresh Read and independent resource authorization.
type Credential struct{ token string }

// ParseCredential accepts only canonical 256-bit base64url credentials.
func ParseCredential(raw string) (Credential, error) {
	if len(raw) != 43 {
		return Credential{}, ErrInvalid
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(b) != 32 || base64.RawURLEncoding.EncodeToString(b) != raw {
		return Credential{}, ErrInvalid
	}
	return Credential{token: raw}, nil
}

// Value deliberately reveals the secret for a future trusted Cookie adapter.
// Never log it, accept it as an Attempt, or publish it after a failed Check.
func (c Credential) Value() string    { return c.token }
func (c Credential) String() string   { return "[redacted session credential]" }
func (c Credential) GoString() string { return c.String() }

// Attempt is immutable, sensitive server-owned admission data, not browser input.
// Its presence is not proof of admission. Only successful Check yields a usable
// Credential; errors never authorize publication. Cross-request persistence and
// process-crash recovery are not implemented by this in-memory value.
type Attempt struct {
	namespace                  string
	credential                 Credential
	state                      identity.SessionState
	created, absolute, initial int64
}

func (a Attempt) String() string   { return "[redacted session attempt]" }
func (a Attempt) GoString() string { return a.String() }

func (a *Attempt) valid(namespace string, p Policy) bool {
	if a == nil || a.namespace != namespace || !identity.ValidID(a.state.ID) || !a.state.Enabled ||
		!a.state.Valid() || !identity.ValidRecoveryID(a.state.RecoveryID) {
		return false
	}
	if _, err := ParseCredential(a.credential.token); err != nil {
		return false
	}
	return a.created > 0 && a.absolute < 9007199254740991 && a.absolute-a.created == p.Absolute.Milliseconds() && a.initial-a.created == p.Idle.Milliseconds()
}

// View is a point-in-time identity observation, not a session credential or
// durable permission. Read does not renew it; accepted in-flight work is not undone.
type View struct {
	SubjectID                                  string
	AuthGeneration                             int64
	RecoveryID                                 string
	CreatedAt, AbsoluteDeadline, IdleExpiresAt time.Time
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type storageError struct{ cause error }

func (e storageError) Error() string        { return ErrStore.Error() }
func (e storageError) Unwrap() error        { return e.cause }
func (e storageError) Is(target error) bool { return target == ErrStore }

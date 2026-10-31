package identity

import "errors"

var (
	ErrRecovering     = errors.New("session recovery authority unavailable")
	ErrRecoveryRandom = errors.New("session recovery generation failed")
)

// ValidRecoveryID accepts an independently random 128-bit lower-hex epoch.
// Its shape is not proof of randomness, maintenance approval, or authorization.
func ValidRecoveryID(generation string) bool { return idPattern.MatchString(generation) }

// RecoveryState is point-in-time maintenance metadata. Empty generation is only
// the initial closed placeholder. A successful read never grants a session.
type RecoveryState struct {
	Generation string
	Enabled    bool
}

// SessionVersion is a server-owned existing-session fence, never client input.
type SessionVersion struct {
	AuthGeneration int64
	RecoveryID     string
}

// Valid rejects missing account or recovery preconditions.
func (v SessionVersion) Valid() bool {
	return v.AuthGeneration > 0 && ValidRecoveryID(v.RecoveryID)
}

// SessionState carries both locked authorities into a trusted storage operation.
// It is not a reusable proof and does not include activation or resource rights.
type SessionState struct {
	State
	RecoveryID string
}

package password

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"
)

var (
	// ErrAcceptance reports invalid or unavailable explicit acceptance rules.
	ErrAcceptance = errors.New("invalid new password acceptance configuration")
	// ErrNewPasswordLength reports invalid UTF-8, insufficient code points or
	// excess bytes. It never includes the supplied password or rule material.
	ErrNewPasswordLength = errors.New("new password length not accepted")
	// ErrWeakPassword reports an exact full-password blocklist match.
	ErrWeakPassword = errors.New("new password not accepted")
)

const maxBlockedPasswords = 16384

// Acceptance holds immutable explicit new-password rules. It is a whole-input
// SHA-256 lookup, not credential storage, a KDF, a breach service or approval of
// dictionary coverage. It neither normalizes nor trims, truncates or changes
// existing-password verification. Callers choose the product minimum and supply
// the same maximum-byte budget as their Hasher.
type Acceptance struct {
	minimum     int
	maxBytes    uint32
	blocked     map[[32]byte]struct{}
	fingerprint [32]byte
}

// NewAcceptance copies a nonempty unique list of nonzero full-input digests.
// Minimum is explicitly 1..64 code points, with room for four bytes per minimum
// code point. No production minimum or blocklist receives a default.
func NewAcceptance(minimum int, maxBytes uint32, blocked [][32]byte) (*Acceptance, error) {
	if minimum < 1 || minimum > 64 || uint64(maxBytes) < 4*uint64(minimum) ||
		uint64(maxBytes) > uint64(^uint(0)>>1) || len(blocked) == 0 || len(blocked) > maxBlockedPasswords {
		return nil, ErrAcceptance
	}
	copy := make(map[[32]byte]struct{}, len(blocked))
	for _, digest := range blocked {
		if digest == ([32]byte{}) {
			return nil, ErrAcceptance
		}
		if _, exists := copy[digest]; exists {
			return nil, ErrAcceptance
		}
		copy[digest] = struct{}{}
	}
	fingerprint := acceptanceFingerprint(minimum, maxBytes, blocked)
	if fingerprint == ([32]byte{}) {
		return nil, ErrAcceptance
	}
	return &Acceptance{minimum: minimum, maxBytes: maxBytes, blocked: copy, fingerprint: fingerprint}, nil
}

// Fingerprint is a stable, nonzero commitment to the complete explicit rules.
// Nil and zero values return zero. This identifies startup metadata; it is not
// a bucket key, credential, authorization result or dictionary coverage proof.
func (a *Acceptance) Fingerprint() [32]byte {
	if a == nil {
		return [32]byte{}
	}
	return a.fingerprint
}

// MaxBytes reports the frozen full-input budget, or zero for invalid rules.
func (a *Acceptance) MaxBytes() uint32 {
	if a == nil || a.fingerprint == ([32]byte{}) {
		return 0
	}
	return a.maxBytes
}

// MinimumCharacters reports the frozen code-point floor, or zero if invalid.
func (a *Acceptance) MinimumCharacters() int {
	if a == nil || a.fingerprint == ([32]byte{}) {
		return 0
	}
	return a.minimum
}

func acceptanceFingerprint(minimum int, maxBytes uint32, blocked [][32]byte) [32]byte {
	ordered := slices.Clone(blocked)
	slices.SortFunc(ordered, func(a, b [32]byte) int { return bytes.Compare(a[:], b[:]) })
	hash := sha256.New()
	_, _ = hash.Write([]byte("oj-new-password-acceptance-v1\x00"))
	// The versioned domain is followed by three uint32 big-endian fields:
	// minimum code points, maximum bytes, entry count; then sorted 32-byte
	// digests. Every field boundary is fixed, including the complete set size.
	var header [12]byte
	binary.BigEndian.PutUint32(header[0:4], uint32(minimum))
	binary.BigEndian.PutUint32(header[4:8], maxBytes)
	binary.BigEndian.PutUint32(header[8:12], uint32(len(ordered)))
	_, _ = hash.Write(header[:])
	for _, digest := range ordered {
		_, _ = hash.Write(digest[:])
	}
	var fingerprint [32]byte
	copy(fingerprint[:], hash.Sum(nil))
	return fingerprint
}

// Check validates full raw UTF-8 input against the explicit length and digest
// rules, without mutation or I/O. Code points are counted without normalization;
// combining sequences and canonically equivalent byte strings remain distinct.
func (a *Acceptance) Check(secret []byte) error {
	if a == nil || a.fingerprint == ([32]byte{}) || a.minimum < 1 || a.minimum > 64 || len(a.blocked) == 0 ||
		uint64(a.maxBytes) < 4*uint64(a.minimum) {
		return ErrAcceptance
	}
	if uint64(len(secret)) > uint64(a.maxBytes) || !utf8.Valid(secret) || utf8.RuneCount(secret) < a.minimum {
		return ErrNewPasswordLength
	}
	if _, blocked := a.blocked[sha256.Sum256(secret)]; blocked {
		return ErrWeakPassword
	}
	return nil
}

func (Acceptance) String() string     { return "[redacted new password acceptance rules]" }
func (a Acceptance) GoString() string { return a.String() }

// Format prevents formatting a rule value or container from dumping its map.
// fmt's built-in type/pointer verbs expose only type/address, never rule data.
func (a Acceptance) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(a.String())) }

// MarshalJSON redacts the rules even when nested in JSON containers.
func (a Acceptance) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted new password acceptance rules]"`), nil
}

// Package emailtoken protects one-time email credentials and their short-lived
// mail payloads. Storage owns eligibility, expiry and atomic consumption.
package emailtoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// Purpose separates activation from password recovery.
type Purpose string

const (
	Activation Purpose = "activation"
	Recovery   Purpose = "password-recovery"

	// MaxSendWindow is independent of the time an already sent link is valid.
	MaxSendWindow = 10 * time.Minute
)

// ErrInvalid is returned for invalid syntax, scope, expiry or authentication.
var ErrInvalid = errors.New("email token data unavailable")

// Lifetime reports the default validity of a newly issued link.
func (p Purpose) Lifetime() time.Duration {
	switch p {
	case Activation:
		return 24 * time.Hour
	case Recovery:
		return 30 * time.Minute
	default:
		return 0
	}
}

// Token is a sensitive, opaque credential. Its zero value is invalid.
type Token struct{ value [32]byte }

// New generates a 256-bit one-time credential using the system random source.
func New() (Token, error) {
	var t Token
	if _, err := rand.Read(t.value[:]); err != nil {
		return Token{}, ErrInvalid
	}
	if !t.valid() {
		return Token{}, ErrInvalid
	}
	return t, nil
}

// Parse validates canonical unpadded base64url syntax, not account authority.
func Parse(encoded string) (Token, error) {
	var t Token
	if len(encoded) != base64.RawURLEncoding.EncodedLen(len(t.value)) {
		return Token{}, ErrInvalid
	}
	n, err := base64.RawURLEncoding.Strict().Decode(t.value[:], []byte(encoded))
	if err != nil || n != len(t.value) || !t.valid() || t.LinkValue() != encoded {
		return Token{}, ErrInvalid
	}
	return t, nil
}

func (t Token) valid() bool { return t.value != [32]byte{} }

// LinkValue reveals the credential only for a trusted email link renderer.
func (t Token) LinkValue() string {
	if !t.valid() {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(t.value[:])
}

// Digest binds the lookup digest to the intended use. The store must still
// verify ownership, validity, recovery epoch and single consumption.
func (t Token) Digest(p Purpose) ([32]byte, error) {
	if !t.valid() || p.Lifetime() == 0 {
		return [32]byte{}, ErrInvalid
	}
	var buffer [80]byte
	data := append(buffer[:0], "oj.email-token.v1\x00"...)
	data = append(data, string(p)...)
	data = append(data, 0)
	data = append(data, t.value[:]...)
	defer clear(data)
	return sha256.Sum256(data), nil
}

// String redacts the credential for diagnostics.
func (Token) String() string { return "[redacted email token]" }

// GoString redacts the credential for Go-syntax diagnostics.
func (t Token) GoString() string { return t.String() }

// Format redacts direct diagnostics even for verbs that bypass String.
func (t Token) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(t.String())) }

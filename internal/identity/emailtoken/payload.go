package emailtoken

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"regexp"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

var keyIDPattern = regexp.MustCompile(`^[a-z0-9_-]{1,16}$`)

const maxProtectorKeys = 4

// Scope is trusted persisted metadata for a single mail job. The worker must
// confirm account and token state against the database before sending. All
// three timestamps use PostgreSQL microsecond precision.
type Scope struct {
	PayloadID, SubjectID, RecoveryID string
	AuthGeneration                   int64
	Recipient                        identity.Email
	Purpose                          Purpose
	IssuedAt, SendUntil, TokenExpiry time.Time
}

func (s Scope) validAt(now time.Time) bool {
	lifetime := s.Purpose.Lifetime()
	return lifetime != 0 && s.AuthGeneration > 0 && identity.ValidID(s.PayloadID) && identity.ValidID(s.SubjectID) &&
		identity.ValidRecoveryID(s.RecoveryID) && s.Recipient.Valid() && !s.IssuedAt.IsZero() &&
		s.IssuedAt.Nanosecond()%1000 == 0 && s.SendUntil.Nanosecond()%1000 == 0 && s.TokenExpiry.Nanosecond()%1000 == 0 &&
		!now.Before(s.IssuedAt) && now.Before(s.SendUntil) && s.IssuedAt.Before(s.SendUntil) &&
		!s.SendUntil.After(s.IssuedAt.Add(MaxSendWindow)) && !s.SendUntil.After(s.TokenExpiry) &&
		!s.TokenExpiry.After(s.IssuedAt.Add(lifetime))
}

// Payload contains only ciphertext and its key identifier.
type Payload struct {
	KeyID      string
	Ciphertext []byte
}

// String redacts the encrypted payload in diagnostics.
func (Payload) String() string { return "[redacted email payload]" }

// GoString redacts the encrypted payload in Go-syntax diagnostics.
func (p Payload) GoString() string { return p.String() }

// Format redacts the encrypted payload for supported diagnostic formats.
func (p Payload) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(p.String())) }

// MarshalJSON redacts the payload. Store the explicit fields in the private
// persistence adapter rather than serializing this value.
func (Payload) MarshalJSON() ([]byte, error) { return []byte(`"[redacted email payload]"`), nil }

// Protector seals new payloads with the current key and opens old ones during
// an explicit rotation overlap. Callers provision independent file-backed keys
// and rotate before reaching 2^32 seals per key across all replicas.
type Protector struct {
	current string
	keys    map[string]cipher.AEAD
}

// CurrentKeyID identifies the immutable active key for aggregate usage admission.
func (p *Protector) CurrentKeyID() string {
	if p == nil {
		return ""
	}
	return p.current
}

// Format prevents diagnostic formatting from inspecting the key ring.
func (Protector) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte("[redacted email payload protector]"))
}

// NewProtector accepts up to four nonzero, distinct AES-256 keys.
func NewProtector(current string, keys map[string][32]byte) (*Protector, error) {
	if len(keys) < 1 || len(keys) > maxProtectorKeys || keys[current] == [32]byte{} {
		return nil, ErrInvalid
	}
	result := &Protector{current: current, keys: make(map[string]cipher.AEAD, len(keys))}
	seen := make(map[[32]byte]bool, len(keys))
	for id, key := range keys {
		if !keyIDPattern.MatchString(id) || key == [32]byte{} || seen[key] {
			return nil, ErrInvalid
		}
		seen[key] = true
		block, err := aes.NewCipher(key[:])
		if err != nil {
			return nil, ErrInvalid
		}
		aead, err := cipher.NewGCMWithRandomNonce(block)
		if err != nil {
			return nil, ErrInvalid
		}
		result.keys[id] = aead
	}
	return result, nil
}

// Seal authenticates the immutable metadata alongside the token.
func (p *Protector) Seal(now time.Time, scope Scope, token Token) (Payload, error) {
	if p == nil || p.keys[p.current] == nil || !scope.validAt(now) || !token.valid() {
		return Payload{}, ErrInvalid
	}
	aead := p.keys[p.current]
	return Payload{KeyID: p.current, Ciphertext: aead.Seal(nil, nil, token.value[:], scope.aad(p.current))}, nil
}

// Open fails closed for unknown keys, wrong bindings and expired send windows.
func (p *Protector) Open(now time.Time, scope Scope, payload Payload) (Token, error) {
	if p == nil || !scope.validAt(now) {
		return Token{}, ErrInvalid
	}
	aead := p.keys[payload.KeyID]
	var t Token
	if aead == nil || len(payload.Ciphertext) != len(t.value)+aead.Overhead() {
		return Token{}, ErrInvalid
	}
	plain, err := aead.Open(nil, nil, payload.Ciphertext, scope.aad(payload.KeyID))
	if err != nil {
		return Token{}, ErrInvalid
	}
	defer clear(plain)
	copy(t.value[:], plain)
	if !t.valid() {
		return Token{}, ErrInvalid
	}
	return t, nil
}

func (s Scope) aad(keyID string) []byte {
	var data []byte
	for _, value := range []string{"oj.email-payload.v1", keyID, s.PayloadID, s.SubjectID, s.RecoveryID,
		s.Recipient.Original(), string(s.Purpose)} {
		data = binary.AppendUvarint(data, uint64(len(value)))
		data = append(data, value...)
	}
	for _, instant := range []time.Time{s.IssuedAt, s.SendUntil, s.TokenExpiry} {
		data = binary.BigEndian.AppendUint64(data, uint64(instant.Unix()))
		data = binary.BigEndian.AppendUint32(data, uint32(instant.Nanosecond()))
	}
	return binary.BigEndian.AppendUint64(data, uint64(s.AuthGeneration))
}

// Package submission owns submission acceptance and its public application contract.
package submission

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxSourceBytes bounds single-file source before persistence.
const MaxSourceBytes = 65536

var (
	ErrInvalid  = errors.New("invalid submission")
	ErrConflict = errors.New("idempotency key already used with different content")
	ErrNotFound = errors.New("submission not found")
	ErrMaterial = errors.New("material unavailable or inconsistent")
)

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

// Request pins a source, language and immutable problem version.
type Request struct {
	MaterialVersion string `json:"material_version"`
	Language        string `json:"language"`
	Source          string `json:"source"`
}

// Receipt reports durable acceptance; queued never implies a verdict.
type Receipt struct {
	ID              string    `json:"id"`
	MaterialVersion string    `json:"material_version"`
	Language        string    `json:"language"`
	CreatedAt       time.Time `json:"created_at"`
	State           string    `json:"state"`
	Verdict         *string   `json:"verdict"`
}

// Store keeps transaction semantics below the application and HTTP boundaries.
type Store interface {
	Accept(context.Context, string, string, string, Request) (Receipt, bool, error)
	Receipt(context.Context, string, string, string) (Receipt, error)
	Get(context.Context, string, string, string) (Receipt, error)
}

// ValidKey constrains keys to a portable, bounded HTTP header value.
func ValidKey(key string) bool { return keyPattern.MatchString(key) }

// Validate checks transport-independent source and identifier constraints.
func Validate(subject, scope, key string, req Request) error {
	if subject == "" || len(subject) > 128 || scope == "" || len(scope) > 128 || !ValidKey(key) ||
		req.MaterialVersion == "" || len(req.MaterialVersion) > 128 || req.Language != "gnu++17" ||
		len(req.Source) == 0 || len(req.Source) > MaxSourceBytes || !utf8.ValidString(req.Source) || strings.ContainsRune(req.Source, 0) {
		return ErrInvalid
	}
	return nil
}

// Digest hashes semantic JSON fields, preserving source bytes and ignoring JSON formatting.
func Digest(req Request) string {
	data, _ := json.Marshal(req)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

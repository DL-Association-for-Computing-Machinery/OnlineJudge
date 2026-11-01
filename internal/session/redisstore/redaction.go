package redisstore

import "fmt"

// These methods protect direct values and pointers for supported legal value
// formats, including numeric verbs which bypass Stringer. Go's invalid-format
// diagnostics, reflection, rewrapping private fields and explicit Value/field
// access can bypass methods. Sensitive objects must still never enter logs.

// Format writes only the credential's fixed redaction label.
func (c Credential) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(c.String())) }

// Format writes only the frozen attempt's fixed redaction label.
func (a Attempt) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(a.String())) }

// Format writes only the continuation's fixed redaction label.
func (c Continuation) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(c.String())) }

// Format writes only the loaded secrets' fixed redaction label.
func (s LoginSecrets) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(s.String())) }

// Format writes only the sealing configuration's fixed redaction label.
func (c LoginConfig) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(c.String())) }

// Format writes only the login manager's fixed redaction label.
func (l Login) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(l.String())) }

// MarshalJSON deliberately hides the sealing key map. LoginConfig is trusted Go
// constructor input, not the independent private key-file or sealed wire schema.
func (LoginConfig) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted login sealing configuration]"`), nil
}

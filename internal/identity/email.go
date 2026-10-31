package identity

import (
	"net/mail"
	"strings"
)

// Email is an accepted ASCII bare address. The entire address is matched without
// case; the exact accepted input is retained for display and future delivery.
// It does not establish ownership, deliverability or permission to log the value.
type Email struct {
	original string
	key      string
}

// ParseEmail accepts an unquoted local part and DNS-style domain labels. It
// rejects headers, comments, whitespace, address literals and internationalized
// addresses rather than silently rewriting them. No DNS lookup is performed.
func ParseEmail(raw string) (Email, error) {
	if len(raw) == 0 || len(raw) > 254 {
		return Email{}, ErrInvalid
	}
	for i := range len(raw) {
		if raw[i] < '!' || raw[i] > '~' {
			return Email{}, ErrInvalid
		}
	}
	if strings.ContainsAny(raw, "\"\\()<>[],:;") {
		return Email{}, ErrInvalid
	}
	address, err := mail.ParseAddress(raw)
	if err != nil || address.Name != "" || address.Address != raw {
		return Email{}, ErrInvalid
	}
	local, domain, ok := strings.Cut(raw, "@")
	if !ok || len(local) == 0 || len(local) > 64 || len(domain) == 0 || len(domain) > 253 {
		return Email{}, ErrInvalid
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return Email{}, ErrInvalid
		}
		for i := range len(label) {
			c := label[i]
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-':
			default:
				return Email{}, ErrInvalid
			}
		}
	}
	return Email{original: raw, key: strings.ToLower(raw)}, nil
}

// Original returns the exact accepted address, preserving its case.
func (e Email) Original() string { return e.original }

// Key returns the case-insensitive identity key. Dots and plus tags are retained.
func (e Email) Key() string { return e.key }

// Valid rejects the zero value and inconsistent internal representations.
func (e Email) Valid() bool {
	parsed, err := ParseEmail(e.original)
	return err == nil && parsed == e
}

// Personal is non-password account metadata. EmailVerified and Enabled are
// independent facts; neither a lookup nor initialization grants training access.
type Personal struct {
	State
	Email         Email
	EmailVerified bool
}

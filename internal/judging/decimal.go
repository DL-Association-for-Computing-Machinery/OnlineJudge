package judging

import (
	"bytes"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// DecimalVersion pins the grammar, arithmetic and resource limits together.
const DecimalVersion = "decimal-tokens-v1"

const (
	decimalBytes      = 256 << 10
	decimalTokenBytes = 128
	decimalTokens     = 4096
	decimalExponent   = 512
)

// DecimalPolicy belongs to an immutable problem version. Empty tolerances are
// disabled; "0" is an explicit exact tolerance. Text lists literal reference
// labels, so a misspelled reference number cannot become an implicit label.
type DecimalPolicy struct {
	Absolute      string   `json:"absolute,omitempty"`
	CaseSensitive bool     `json:"case_sensitive,omitempty"`
	Relative      string   `json:"relative,omitempty"`
	Text          []string `json:"text,omitempty"`
}

var decimalSyntax = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

func parseDecimal(s string) (*big.Rat, bool) {
	if len(s) == 0 || len(s) > decimalTokenBytes || !decimalSyntax.MatchString(s) {
		return nil, false
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		e, err := strconv.Atoi(s[i+1:])
		if err != nil || e < -decimalExponent || e > decimalExponent {
			return nil, false
		}
	}
	return new(big.Rat).SetString(s)
}

func (p DecimalPolicy) tolerances() (a, r *big.Rat, ok bool) {
	for _, v := range []struct {
		token string
		dest  **big.Rat
	}{{p.Absolute, &a}, {p.Relative, &r}} {
		if v.token != "" {
			n, parsed := parseDecimal(v.token)
			if !parsed || n.Sign() < 0 {
				return nil, nil, false
			}
			*v.dest = n
		}
	}
	if a == nil && r == nil || len(p.Text) > 64 {
		return nil, nil, false
	}
	for i, s := range p.Text {
		if s == "" || len(s) > decimalTokenBytes || !utf8.ValidString(s) ||
			strings.IndexByte(s, 0) >= 0 || strings.IndexFunc(s, func(c rune) bool { return c < 128 && asciiSpace(byte(c)) }) >= 0 ||
			decimalSyntax.MatchString(s) || i > 0 && p.Text[i-1] >= s {
			return nil, nil, false
		}
	}
	return a, r, true
}

// Valid checks the explicit tolerance and reference label contract.
func (p DecimalPolicy) Valid() bool {
	_, _, ok := p.tolerances()
	return ok
}

// ValidateDecimalReference rejects corrupt material even if a contestant fails
// at an earlier token or point.
func ValidateDecimalReference(reference []byte, policy DecimalPolicy) error {
	if !policy.Valid() || len(reference) > decimalBytes || !utf8.Valid(reference) {
		return ErrReference
	}
	count := 0
	for token, rest := nextToken(reference, asciiSpace); len(token) != 0; token, rest = nextToken(rest, asciiSpace) {
		count++
		if count > decimalTokens || len(token) > decimalTokenBytes {
			return ErrReference
		}
		if _, ok := slices.BinarySearch(policy.Text, string(token)); !ok {
			if _, ok := parseDecimal(string(token)); !ok {
				return ErrReference
			}
		}
	}
	if count == 0 {
		return ErrReference
	}
	return nil
}

// CompareDecimal compares ordered tokens without rounding tolerance boundaries.
func CompareDecimal(reference, output []byte, policy DecimalPolicy) (bool, error) {
	if err := ValidateDecimalReference(reference, policy); err != nil {
		return false, err
	}
	if len(output) > decimalBytes {
		return false, ErrOutput
	}
	if !utf8.Valid(output) {
		return false, nil
	}
	a, r, _ := policy.tolerances()
	for {
		var expected, actual []byte
		expected, reference = nextToken(reference, asciiSpace)
		actual, output = nextToken(output, asciiSpace)
		if len(expected) == 0 || len(actual) == 0 {
			return len(expected) == len(actual), nil
		}
		if bytes.Equal(expected, actual) {
			continue
		}
		if _, label := slices.BinarySearch(policy.Text, string(expected)); label {
			if policy.CaseSensitive || !equalASCIIFold(expected, actual) {
				return false, nil
			}
			continue
		}
		want, _ := parseDecimal(string(expected))
		got, ok := parseDecimal(string(actual))
		if !ok {
			return false, nil
		}
		difference := new(big.Rat).Abs(got.Sub(got, want))
		if (a != nil && difference.Cmp(a) <= 0) || (r != nil && difference.Cmp(want.Abs(want).Mul(want, r)) <= 0) {
			continue
		}
		return false, nil
	}
}

func equalASCIIFold(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i, x := range a {
		y := b[i]
		if x >= 'A' && x <= 'Z' {
			x |= 32
		}
		if y >= 'A' && y <= 'Z' {
			y |= 32
		}
		if x != y {
			return false
		}
	}
	return true
}

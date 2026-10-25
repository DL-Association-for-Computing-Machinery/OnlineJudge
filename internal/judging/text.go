// Package judging contains deterministic judging rules, independent of sandbox I/O.
package judging

import (
	"errors"
	"unicode/utf8"
)

var (
	ErrReference = errors.New("invalid or oversized reference answer")
	ErrOutput    = errors.New("contestant output exceeds checker budget")
)

// TextBudget is the M0 checker byte budget; deployment limits must be validated separately.
const TextBudget = 1 << 20
const tokenBudget = 1 << 16

// CompareText compares ordered ASCII-whitespace-delimited UTF-8 tokens, folding
// only A-Z unless caseSensitive is set. It never normalizes numbers or Unicode.
func CompareText(reference, output []byte, caseSensitive bool) (bool, error) {
	if len(reference) > TextBudget || !utf8.Valid(reference) {
		return false, ErrReference
	}
	if len(output) > TextBudget {
		return false, ErrOutput
	}
	// Validate every reference token before considering any contestant mismatch.
	for rest := reference; len(rest) > 0; {
		token, next := nextToken(rest)
		rest = next
		if len(token) > tokenBudget {
			return false, ErrReference
		}
	}
	if !utf8.Valid(output) {
		return false, nil
	}
	for rest := output; len(rest) > 0; {
		token, next := nextToken(rest)
		rest = next
		if len(token) > tokenBudget {
			return false, ErrOutput
		}
	}
	for {
		var a, b []byte
		a, reference = nextToken(reference)
		b, output = nextToken(output)
		if len(a) == 0 || len(b) == 0 {
			return len(a) == len(b), nil
		}
		if len(a) != len(b) {
			return false, nil
		}
		for i := range a {
			x, y := a[i], b[i]
			if !caseSensitive {
				if x >= 'A' && x <= 'Z' {
					x += 'a' - 'A'
				}
				if y >= 'A' && y <= 'Z' {
					y += 'a' - 'A'
				}
			}
			if x != y {
				return false, nil
			}
		}
	}
}

func nextToken(data []byte) ([]byte, []byte) {
	space := func(b byte) bool { return b == ' ' || b >= '\t' && b <= '\r' }
	i := 0
	for i < len(data) && space(data[i]) {
		i++
	}
	data = data[i:]
	i = 0
	for i < len(data) && !space(data[i]) {
		i++
	}
	return data[:i], data[i:]
}

package judging

import "errors"

// LibraryCheckerTextVersion identifies the supported UTF-8 subset of the pinned
// Library Checker wcmp semantics, separately from the M0 ASCII whitespace rules.
const LibraryCheckerTextVersion = "lc-wcmp-tokens-v1"

// MaxTextBytes is the component ceiling, not a production execution limit or a
// guarantee about memory already allocated by the caller.
const MaxTextBytes = 128 << 20

// ErrTextLimits rejects absent, excessive or inconsistent checker limits.
var ErrTextLimits = errors.New("invalid text checker limits")

// TextLimits bounds each reference/output and each token independently. Callers
// must bound material reads before allocating the byte slices supplied here.
type TextLimits struct {
	Bytes      int
	TokenBytes int
}

// CompareLibraryCheckerText compares case-sensitive token bytes without numeric
// equivalence. Only SP, TAB, LF and CR separate tokens, as in the pinned wcmp.
// The supported domain is UTF-8 with tokens up to 64 KiB and explicit per-file
// limits up to MaxTextBytes. Invalid references return ErrReference. Output byte
// overflow returns ErrOutput, then invalid UTF-8 is rejected as a mismatch,
// then valid UTF-8 output token overflow returns ErrOutput.
// No Testlib execution, draft validation, or publication takes place here.
func CompareLibraryCheckerText(reference, output []byte, limits TextLimits) (bool, error) {
	if limits.Bytes < 1 || limits.Bytes > MaxTextBytes || limits.TokenBytes < 1 || limits.TokenBytes > tokenBudget || limits.TokenBytes > limits.Bytes {
		return false, ErrTextLimits
	}
	return compareText(reference, output, true, limits.Bytes, limits.TokenBytes, libraryCheckerSpace)
}

func libraryCheckerSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

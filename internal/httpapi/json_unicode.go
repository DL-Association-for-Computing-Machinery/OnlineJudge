package httpapi

import (
	"encoding/json"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// encoding/json replaces unpaired UTF-16 surrogates with U+FFFD. Reject them
// before decoding so the accepted source and its digest cannot silently change.
// The standard library validates JSON grammar; this scan only checks pairing.
func validJSONUnicode(data []byte) bool {
	if !utf8.Valid(data) || !json.Valid(data) {
		return false
	}
	for i := 0; i < len(data); i++ {
		if data[i] != '\\' {
			continue
		}
		i++
		if data[i] != 'u' {
			continue // Includes escaped backslashes, which must not start a pair.
		}
		unit, _ := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		i += 4
		if !utf16.IsSurrogate(rune(unit)) {
			continue
		}
		if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
			return false
		}
		next, _ := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
		if utf16.DecodeRune(rune(unit), rune(next)) == utf8.RuneError {
			return false
		}
		i += 6
	}
	return true
}

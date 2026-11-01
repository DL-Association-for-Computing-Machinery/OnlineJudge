package password

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/jsoninput"
)

const maxAcceptanceBytes = 1 << 20

// ParseAcceptance reads one bounded nonsecret digest-list document with no I/O.
// Its only fields are the integer lexical version 1 and a nonempty sha256 array
// of canonical lowercase 64-hex digests. Errors contain no decoder/input cause.
// The independent file byte budget may limit list size before the entry ceiling.
func ParseAcceptance(body []byte, minimum int, maxBytes uint32) (*Acceptance, error) {
	if len(body) == 0 || len(body) > maxAcceptanceBytes || !jsoninput.ValidUnicode(body) {
		return nil, ErrAcceptance
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if !acceptanceToken(decoder, json.Delim('{')) {
		return nil, ErrAcceptance
	}
	versionSeen, listSeen := false, false
	var blocked [][32]byte
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return nil, ErrAcceptance
		}
		switch key {
		case "version":
			if versionSeen || !acceptanceToken(decoder, json.Number("1")) {
				return nil, ErrAcceptance
			}
			versionSeen = true
		case "sha256":
			if listSeen {
				return nil, ErrAcceptance
			}
			listSeen = true
			blocked, err = acceptanceDigests(decoder)
			if err != nil {
				return nil, ErrAcceptance
			}
		default:
			return nil, ErrAcceptance
		}
	}
	if !versionSeen || !listSeen || !acceptanceToken(decoder, json.Delim('}')) {
		return nil, ErrAcceptance
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrAcceptance
	}
	return NewAcceptance(minimum, maxBytes, blocked)
}

func acceptanceToken(decoder *json.Decoder, want json.Token) bool {
	got, err := decoder.Token()
	return err == nil && got == want
}

func acceptanceDigests(decoder *json.Decoder) ([][32]byte, error) {
	if !acceptanceToken(decoder, json.Delim('[')) {
		return nil, ErrAcceptance
	}
	blocked := make([][32]byte, 0)
	for decoder.More() {
		if len(blocked) >= maxBlockedPasswords {
			return nil, ErrAcceptance
		}
		token, err := decoder.Token()
		text, ok := token.(string)
		if err != nil || !ok || len(text) != 64 || strings.ToLower(text) != text {
			return nil, ErrAcceptance
		}
		var digest [32]byte
		if _, err := hex.Decode(digest[:], []byte(text)); err != nil {
			return nil, ErrAcceptance
		}
		blocked = append(blocked, digest)
	}
	if !acceptanceToken(decoder, json.Delim(']')) {
		return nil, ErrAcceptance
	}
	return blocked, nil
}

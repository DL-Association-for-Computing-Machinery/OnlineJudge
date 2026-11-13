package emailtoken

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/jsoninput"
)

// ErrKeyFile omits paths, decoder errors and key material.
var ErrKeyFile = errors.New("email payload key file unavailable or invalid")

const (
	maxKeyFileBytes = 4096
	keyFileFormat   = "oj.email-payload-keys.v1"
)

// LoadProtector reads an owner-private email payload key file and returns a
// protector snapshot. It does not generate keys or reload a live protector.
func LoadProtector(path string) (*Protector, error) {
	body, err := checkedfile.ReadSecret(path, maxKeyFileBytes)
	if err != nil {
		return nil, ErrKeyFile
	}
	defer clear(body)
	return parseKeyFile(body)
}

func parseKeyFile(body []byte) (*Protector, error) {
	if !jsoninput.ValidUnicode(body) {
		return nil, ErrKeyFile
	}
	d := json.NewDecoder(bytes.NewReader(body))
	if start, err := d.Token(); err != nil || start != json.Delim('{') {
		return nil, ErrKeyFile
	}
	var current string
	var keys map[string][32]byte
	seen := make(map[string]bool, 3)
	for d.More() {
		field, err := d.Token()
		name, ok := field.(string)
		if err != nil || !ok || seen[name] {
			return nil, ErrKeyFile
		}
		seen[name] = true
		switch name {
		case "format":
			if format, err := d.Token(); err != nil || format != keyFileFormat {
				return nil, ErrKeyFile
			}
		case "current":
			id, err := d.Token()
			var ok bool
			if current, ok = id.(string); err != nil || !ok {
				return nil, ErrKeyFile
			}
		case "keys":
			keys, err = readKeyFileKeys(d)
			if err != nil {
				return nil, ErrKeyFile
			}
		default:
			return nil, ErrKeyFile
		}
	}
	if end, err := d.Token(); err != nil || end != json.Delim('}') || len(seen) != 3 {
		return nil, ErrKeyFile
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, ErrKeyFile
	}
	protector, err := NewProtector(current, keys)
	if err != nil {
		return nil, ErrKeyFile
	}
	return protector, nil
}

func readKeyFileKeys(d *json.Decoder) (map[string][32]byte, error) {
	if start, err := d.Token(); err != nil || start != json.Delim('{') {
		return nil, ErrKeyFile
	}
	keys := make(map[string][32]byte, maxProtectorKeys)
	for d.More() {
		field, err := d.Token()
		id, ok := field.(string)
		if err != nil || !ok || len(keys) >= maxProtectorKeys {
			return nil, ErrKeyFile
		}
		if _, duplicate := keys[id]; duplicate {
			return nil, ErrKeyFile
		}
		value, err := d.Token()
		encoded, ok := value.(string)
		if err != nil || !ok || len(encoded) != base64.RawURLEncoding.EncodedLen(32) {
			return nil, ErrKeyFile
		}
		// Length and round-trip checks reject padding, line breaks and alternate encodings.
		raw, err := base64.RawURLEncoding.DecodeString(encoded)
		valid := err == nil && len(raw) == 32 && base64.RawURLEncoding.EncodeToString(raw) == encoded
		var key [32]byte
		copy(key[:], raw)
		clear(raw)
		if !valid {
			return nil, ErrKeyFile
		}
		keys[id] = key
	}
	if end, err := d.Token(); err != nil || end != json.Delim('}') {
		return nil, ErrKeyFile
	}
	return keys, nil
}

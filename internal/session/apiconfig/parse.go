package apiconfig

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/jsoninput"
)

const maxBytes = 64 * 1024

// Parse accepts exactly one versioned JSON object and returns only ErrConfig on
// failure. It never evaluates file paths, environment references or TLS files.
func Parse(body []byte) (Config, error) {
	if len(body) == 0 || len(body) > maxBytes || !jsoninput.ValidUnicode(body) {
		return Config{}, ErrConfig
	}
	preflight := json.NewDecoder(bytes.NewReader(body))
	preflight.UseNumber()
	keys := 0
	if scanValue(preflight, 0, &keys) != nil {
		return Config{}, ErrConfig
	}
	if _, err := preflight.Token(); err != io.EOF {
		return Config{}, ErrConfig
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var wire document
	if decoder.Decode(&wire) != nil {
		return Config{}, ErrConfig
	}
	result, err := wire.config()
	if err != nil {
		return Config{}, ErrConfig
	}
	return result, nil
}

// scanValue uses the standard grammar while bounding container work and
// rejecting aliases before encoding/json can silently overwrite earlier keys.
func scanValue(decoder *json.Decoder, depth int, keys *int) error {
	token, err := decoder.Token()
	if err != nil || token == nil {
		return ErrConfig
	}
	delim, container := token.(json.Delim)
	if !container {
		if depth == 0 {
			return ErrConfig
		}
		return nil
	}
	depth++
	if depth > 12 || (depth == 1 && delim != '{') {
		return ErrConfig
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			key, err := decoder.Token()
			name, ok := key.(string)
			if err != nil || !ok || !canonicalKey(name) {
				return ErrConfig
			}
			if _, duplicate := seen[name]; duplicate {
				return ErrConfig
			}
			seen[name] = struct{}{}
			*keys = *keys + 1
			if *keys > 256 || scanValue(decoder, depth, keys) != nil {
				return ErrConfig
			}
		}
	case '[':
		for count := 0; decoder.More(); count++ {
			if count >= 16 || scanValue(decoder, depth, keys) != nil {
				return ErrConfig
			}
		}
	default:
		return ErrConfig
	}
	end, err := decoder.Token()
	if err != nil || (delim == '{' && end != json.Delim('}')) || (delim == '[' && end != json.Delim(']')) {
		return ErrConfig
	}
	return nil
}

func canonicalKey(key string) bool {
	if key == "" || key[0] < 'a' || key[0] > 'z' || key[len(key)-1] == '_' {
		return false
	}
	previousUnderscore := false
	for _, c := range key {
		if c == '_' {
			if previousUnderscore {
				return false
			}
			previousUnderscore = true
			continue
		}
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
		previousUnderscore = false
	}
	return true
}

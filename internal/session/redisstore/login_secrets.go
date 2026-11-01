package redisstore

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/jsoninput"
)

// ErrLoginSecrets never wraps filesystem/decoder errors, paths or secret data.
var ErrLoginSecrets = errors.New("login secrets file unavailable or invalid")

const maxLoginSecretsBytes = 4096

// LoginSecrets is an immutable, opaque secret snapshot. Sealing keys may rotate;
// the distinct limiting key must remain stable across replicas and such rotation.
// It has no secret getters and default JSON encoding exposes no private fields.
type LoginSecrets struct {
	current  string
	sealing  map[string][32]byte
	limiting [32]byte
}

func (s LoginSecrets) String() string   { return "[redacted login secrets]" }
func (s LoginSecrets) GoString() string { return s.String() }

// LoadLoginSecrets reads a bounded, owner-private key file through checked file
// descriptors. It never creates files/keys, changes permissions or reloads a live
// manager. Deploy on a trusted local filesystem with appropriate ACLs; see the
// platform path checks. Tests use only synthetic files, never real user secrets.
func LoadLoginSecrets(path string) (*LoginSecrets, error) {
	body, err := readLoginSecrets(path)
	if err != nil {
		return nil, ErrLoginSecrets
	}
	defer clear(body)
	return parseLoginSecrets(body)
}

// NewLoginFromSecrets connects one loaded snapshot to the existing constructor.
// It neither chooses Origin/Window nor opens routes. The limiting secret is only
// loaded and validated here; distributed admission is configured separately.
func NewLoginFromSecrets(store *Store, origin string, window time.Duration, secrets *LoginSecrets) (*Login, error) {
	if secrets == nil || !validLoginSecrets(secrets) {
		return nil, ErrLoginSecrets
	}
	return NewLogin(store, LoginConfig{Origin: origin, Window: window, CurrentKey: secrets.current, Keys: secrets.sealing})
}

func parseLoginSecrets(body []byte) (*LoginSecrets, error) {
	if len(body) == 0 || len(body) > maxLoginSecretsBytes || !jsoninput.ValidUnicode(body) {
		return nil, ErrLoginSecrets
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return nil, ErrLoginSecrets
	}
	seen := make(map[string]bool, 4)
	s := &LoginSecrets{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, ErrLoginSecrets
		}
		name, ok := token.(string)
		if !ok || seen[name] {
			return nil, ErrLoginSecrets
		}
		seen[name] = true
		switch name {
		case "version":
			v, err := d.Token()
			n, ok := v.(json.Number)
			if err != nil || !ok || n.String() != "1" {
				return nil, ErrLoginSecrets
			}
		case "current":
			v, err := d.Token()
			text, ok := v.(string)
			if err != nil || !ok {
				return nil, ErrLoginSecrets
			}
			s.current = text
		case "sealing":
			keys, err := readSealingKeys(d)
			if err != nil {
				return nil, ErrLoginSecrets
			}
			s.sealing = keys
		case "limiting":
			key, err := readSecret(d)
			if err != nil {
				return nil, ErrLoginSecrets
			}
			s.limiting = key
		default:
			return nil, ErrLoginSecrets
		}
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') || len(seen) != 4 {
		return nil, ErrLoginSecrets
	}
	if _, err = d.Token(); err != io.EOF || !validLoginSecrets(s) {
		return nil, ErrLoginSecrets
	}
	return s, nil
}

func readSealingKeys(d *json.Decoder) (map[string][32]byte, error) {
	v, err := d.Token()
	if err != nil || v != json.Delim('{') {
		return nil, ErrLoginSecrets
	}
	keys := make(map[string][32]byte, 4)
	for d.More() {
		v, err := d.Token()
		id, ok := v.(string)
		if err != nil || !ok || !keyIDPattern.MatchString(id) || len(keys) >= 4 {
			return nil, ErrLoginSecrets
		}
		if _, duplicate := keys[id]; duplicate {
			return nil, ErrLoginSecrets
		}
		key, err := readSecret(d)
		if err != nil {
			return nil, ErrLoginSecrets
		}
		keys[id] = key
	}
	v, err = d.Token()
	if err != nil || v != json.Delim('}') {
		return nil, ErrLoginSecrets
	}
	return keys, nil
}

func readSecret(d *json.Decoder) ([32]byte, error) {
	v, err := d.Token()
	text, ok := v.(string)
	if err != nil || !ok || len(text) != 43 {
		return [32]byte{}, ErrLoginSecrets
	}
	raw, err := base64.RawURLEncoding.DecodeString(text)
	defer clear(raw)
	if err != nil || len(raw) != 32 || base64.RawURLEncoding.EncodeToString(raw) != text {
		return [32]byte{}, ErrLoginSecrets
	}
	var key [32]byte
	copy(key[:], raw)
	if key == [32]byte{} {
		return [32]byte{}, ErrLoginSecrets
	}
	return key, nil
}

func validLoginSecrets(s *LoginSecrets) bool {
	if s == nil || !validSealingKeys(s.current, s.sealing) || s.limiting == [32]byte{} {
		return false
	}
	seen := make(map[[32]byte]bool, len(s.sealing))
	for _, key := range s.sealing {
		if seen[key] || key == s.limiting {
			return false
		}
		seen[key] = true
	}
	return true
}

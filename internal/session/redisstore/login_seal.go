package redisstore

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

var keyIDPattern = regexp.MustCompile(`^[a-z0-9_-]{1,16}$`)

// NewLogin copies caller key material into standard-library AES-256-GCM AEADs.
// Old keys may decrypt during rotation; only CurrentKey seals new candidates.
// This does not provision/read keys, pick production budgets or register routes.
func NewLogin(store *Store, c LoginConfig) (*Login, error) {
	u, err := url.Parse(c.Origin)
	if store == nil || store.client == nil || store.authority == nil || store.gate == nil ||
		!namespacePattern.MatchString(store.namespace) || !store.policy.valid() || err != nil || u.Scheme != "https" || u.Hostname() == "" ||
		c.Origin != "https://"+u.Host || u.User != nil || c.Window < time.Millisecond || c.Window > 5*time.Minute ||
		c.Window > store.policy.Idle || c.Window%time.Millisecond != 0 || !validSealingKeys(c.CurrentKey, c.Keys) {
		return nil, ErrInvalid
	}
	keys := make(map[string]cipher.AEAD, len(c.Keys))
	for id, key := range c.Keys {
		block, err := aes.NewCipher(key[:])
		if err != nil {
			return nil, ErrInvalid
		}
		aead, err := cipher.NewGCMWithRandomNonce(block)
		if err != nil {
			return nil, ErrInvalid
		}
		keys[id] = aead
	}
	return &Login{store: store, origin: c.Origin, window: c.Window, current: c.CurrentKey, keys: keys}, nil
}

func validSealingKeys(current string, keys map[string][32]byte) bool {
	if !keyIDPattern.MatchString(current) || len(keys) < 1 || len(keys) > 4 {
		return false
	}
	for id, key := range keys {
		if !keyIDPattern.MatchString(id) || key == [32]byte{} {
			return false
		}
	}
	return keys[current] != [32]byte{}
}

// ParseContinuation only bounds/canonicalizes browser syntax. Successful parsing
// does not authenticate the envelope, its deadline or a session credential.
func ParseContinuation(value string) (Continuation, error) {
	if len(value) > 1536 {
		return Continuation{}, ErrInvalid
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != "1" || !keyIDPattern.MatchString(parts[1]) {
		return Continuation{}, ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(raw) < 28 || base64.RawURLEncoding.EncodeToString(raw) != parts[2] {
		return Continuation{}, ErrInvalid
	}
	return Continuation{value: value}, nil
}

type loginPayload struct {
	Token                             string `json:"token"`
	Subject                           string `json:"subject"`
	Credential, Auth                  int64
	Recovery                          string `json:"recovery"`
	Created, Absolute, Initial, Until int64
}

func (l *Login) aad(key string) []byte {
	var result []byte
	for _, v := range []string{"oj-login-attempt-v1", key, l.store.namespace, l.origin,
		strconv.FormatInt(l.store.policy.Idle.Milliseconds(), 10), strconv.FormatInt(l.store.policy.Absolute.Milliseconds(), 10), strconv.FormatInt(l.window.Milliseconds(), 10)} {
		result = binary.AppendUvarint(result, uint64(len(v)))
		result = append(result, v...)
	}
	return result
}

func (l *Login) seal(a *Attempt) (Continuation, error) {
	if !l.valid(a) {
		return Continuation{}, ErrInvalid
	}
	p := loginPayload{a.credential.token, a.state.ID, a.state.CredentialVersion, a.state.AuthGeneration, a.state.RecoveryID, a.created, a.absolute, a.initial, a.loginUntil}
	plain, err := json.Marshal(p)
	if err != nil {
		return Continuation{}, ErrInvalid
	}
	defer clear(plain)
	encrypted := l.keys[l.current].Seal(nil, nil, plain, l.aad(l.current))
	value := "1." + l.current + "." + base64.RawURLEncoding.EncodeToString(encrypted)
	c, err := ParseContinuation(value)
	if err != nil {
		return Continuation{}, err
	}
	c.until = time.UnixMilli(a.loginUntil)
	return c, nil
}

func (l *Login) open(c Continuation) (*Attempt, error) {
	if l == nil || l.store == nil {
		return nil, ErrInvalid
	}
	if _, err := ParseContinuation(c.value); err != nil {
		return nil, ErrInvalid
	}
	parts := strings.Split(c.value, ".")
	aead := l.keys[parts[1]]
	if aead == nil {
		return nil, ErrInvalid
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[2])
	plain, err := aead.Open(nil, nil, raw, l.aad(parts[1]))
	if err != nil {
		return nil, ErrInvalid
	}
	defer clear(plain)
	var p loginPayload
	if json.Unmarshal(plain, &p) != nil {
		return nil, ErrInvalid
	}
	a := &Attempt{namespace: l.store.namespace, credential: Credential{token: p.Token}, state: identity.SessionState{
		State: identity.State{ID: p.Subject, Enabled: true, Revision: identity.Revision{CredentialVersion: p.Credential, AuthGeneration: p.Auth}}, RecoveryID: p.Recovery},
		created: p.Created, absolute: p.Absolute, initial: p.Initial, loginUntil: p.Until}
	if !l.valid(a) {
		return nil, ErrInvalid
	}
	return a, nil
}

func (l *Login) valid(a *Attempt) bool {
	return a.valid(l.store.namespace, l.store.policy) && a.loginUntil > a.created && a.loginUntil <= a.initial && a.loginUntil-a.created == l.window.Milliseconds()
}

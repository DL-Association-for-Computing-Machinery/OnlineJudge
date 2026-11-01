package redisstore

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"
)

// ErrLoginRate is a quota denial, distinct from the ordinary session slot limit.
// It reveals neither the exhausted dimension nor any remaining allowance.
var ErrLoginRate = errors.New("login admission rate exceeded")

// LoginBucket requires an explicit fixed-window request budget, not concurrency.
// Count is 1..1,000,000; Window is a whole millisecond in 1ms..24h. These bounds
// are implementation limits, not recommended production values.
type LoginBucket struct {
	Count  int
	Window time.Duration
}

// LoginLimits is shared by every replica of a session namespace. Begin uses
// global/source/canonical-email quotas; complete and abandon share a distinct
// recovery global/source budget. No production defaults or success refunds exist.
type LoginLimits struct {
	BeginGlobal, BeginSource, BeginEmail LoginBucket
	RecoveryGlobal, RecoverySource       LoginBucket
}

func (c LoginLimits) buckets(recovery bool) []LoginBucket {
	if recovery {
		return []LoginBucket{c.RecoveryGlobal, c.RecoverySource}
	}
	return []LoginBucket{c.BeginGlobal, c.BeginSource, c.BeginEmail}
}

func (c LoginLimits) valid() bool {
	for _, b := range append(c.buckets(false), c.buckets(true)...) {
		if b.Count < 1 || b.Count > 1000000 || b.Window < time.Millisecond || b.Window > 24*time.Hour || b.Window%time.Millisecond != 0 {
			return false
		}
	}
	return true
}

type loginAdmission struct {
	store            *Store
	limits           LoginLimits
	key              [32]byte
	prefix, metadata string
}

func newLoginAdmission(store *Store, secrets *LoginSecrets, limits LoginLimits) (*loginAdmission, error) {
	if store == nil || !boundedClient(store.client) || !namespacePattern.MatchString(store.namespace) || !limits.valid() {
		return nil, ErrInvalid
	}
	if !validLoginSecrets(secrets) {
		return nil, ErrLoginSecrets
	}
	a := &loginAdmission{store: store, limits: limits, key: secrets.limiting, prefix: "oj-login-limit:" + store.namespace + ":"}
	config := ""
	for _, b := range append(limits.buckets(false), limits.buckets(true)...) {
		config += strconv.Itoa(b.Count) + "/" + strconv.FormatInt(b.Window.Milliseconds(), 10) + ";"
	}
	a.metadata = "1:" + a.mac("config", config)
	return a, nil
}

func (a *loginAdmission) mac(purpose, value string) string {
	m := hmac.New(sha256.New, a.key[:])
	// Every component is either a fixed label, a validated namespace, or a
	// canonical email/IP with no NUL. This separates configuration and buckets.
	_, _ = m.Write([]byte("oj-login-limit-v1\x00" + a.store.namespace + "\x00" + purpose + "\x00" + value))
	return hex.EncodeToString(m.Sum(nil))
}

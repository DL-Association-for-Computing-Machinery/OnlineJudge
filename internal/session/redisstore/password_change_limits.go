package redisstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

// ErrPasswordChangeRate discloses no exhausted dimension or remaining quota.
var ErrPasswordChangeRate = errors.New("password change admission rate exceeded")

var passwordChangeAdmitLua = strings.Replace(loginAdmitLua, "-- session-protocol:login-admit", "-- session-protocol:password-change-admit", 1)

// PasswordChangeLimits requires three independent fixed-window quotas. Every
// replica shares these values and its separate persistent provisioning marker.
// These are not production defaults and do not change existing login budgets.
type PasswordChangeLimits struct{ Global, Source, Subject LoginBucket }

// Validate checks pure bounds without connecting or provisioning metadata.
func (c PasswordChangeLimits) Validate() error {
	if !c.Global.valid() || !c.Source.valid() || !c.Subject.valid() {
		return ErrInvalid
	}
	return nil
}

func (c PasswordChangeLimits) buckets() []LoginBucket {
	return []LoginBucket{c.Global, c.Source, c.Subject}
}

// PasswordChangeAdmission owns immutable, purpose-separated shared quotas.
// Transports authenticate a session first and pass its trusted subject plus the
// observed source; admission must precede ALL password verification/derivation.
type PasswordChangeAdmission struct {
	store            *Store
	limits           PasswordChangeLimits
	key              [32]byte
	prefix, metadata string
}

func (PasswordChangeAdmission) String() string     { return "[redacted password change admission]" }
func (a PasswordChangeAdmission) GoString() string { return a.String() }
func (a PasswordChangeAdmission) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte(a.String()))
}

func newPasswordChangeAdmission(store *Store, secrets *LoginSecrets, limits PasswordChangeLimits, acceptance *password.Acceptance) (*PasswordChangeAdmission, error) {
	if store == nil || !boundedClient(store.client) || !namespacePattern.MatchString(store.namespace) || limits.Validate() != nil {
		return nil, ErrInvalid
	}
	if !validLoginSecrets(secrets) {
		return nil, ErrLoginSecrets
	}
	fingerprint := acceptance.Fingerprint()
	if fingerprint == ([32]byte{}) {
		return nil, ErrInvalid
	}
	a := &PasswordChangeAdmission{store: store, limits: limits, key: secrets.limiting, prefix: "oj-password-change-limit:" + store.namespace + ":"}
	config := ""
	for _, b := range limits.buckets() {
		config += strconv.Itoa(b.Count) + "/" + strconv.FormatInt(b.Window.Milliseconds(), 10) + ";"
	}
	config += "acceptance:" + hex.EncodeToString(fingerprint[:])
	a.metadata = "1:" + a.mac("config", config)
	return a, nil
}

func (a *PasswordChangeAdmission) mac(purpose, value string) string {
	m := hmac.New(sha256.New, a.key[:])
	_, _ = m.Write([]byte("oj-password-change-limit-v1\x00" + a.store.namespace + "\x00" + purpose + "\x00" + value))
	return hex.EncodeToString(m.Sum(nil))
}

// InitializePasswordChangeLimits is trusted provisioning under deployment
// isolation on a known initial namespace. It is never a startup repair or a
// recovery workflow; lost counters require external isolation/recovery first.
// An existing identical persistent marker is idempotent; mismatch fails closed.
func InitializePasswordChangeLimits(ctx context.Context, store *Store, secrets *LoginSecrets, limits PasswordChangeLimits, acceptance *password.Acceptance) error {
	a, err := newPasswordChangeAdmission(store, secrets, limits, acceptance)
	if err != nil {
		return err
	}
	script := "#!lua flags=no-cluster\n-- session-protocol:password-change-limit-init\n" + `
if redis.call('TYPE', KEYS[1]).ok == 'none' then
    redis.call('SET', KEYS[1], ARGV[1], 'NX')
end
` + loginLimitsMetadataLua
	allowed, err := runAdmission(ctx, store, script, []string{a.prefix + "config"}, []any{a.metadata})
	if err == nil && !allowed {
		return storageError{ErrConflict}
	}
	return err
}

// NewPasswordChangeAdmission only checks the existing marker; it never creates
// it, resets counters or alters login admission. Missing/mismatched data refuses
// construction. The same Store/client/namespace owner serves session checks.
func NewPasswordChangeAdmission(ctx context.Context, store *Store, secrets *LoginSecrets, limits PasswordChangeLimits, acceptance *password.Acceptance) (*PasswordChangeAdmission, error) {
	a, err := newPasswordChangeAdmission(store, secrets, limits, acceptance)
	if err != nil {
		return nil, err
	}
	allowed, err := runAdmission(ctx, store, "#!lua flags=no-writes,no-cluster\n-- session-protocol:password-change-limit-check\n"+loginLimitsMetadataLua, []string{a.prefix + "config"}, []any{a.metadata})
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, storageError{ErrConflict}
	}
	return a, nil
}

// Admit charges global/source/subject together. Rejection writes no dimension
// or TTL; uncertain/partial effects are never retried/refunded. Inputs are from
// the just-authenticated session and trusted transport, never body parameters.
func (a *PasswordChangeAdmission) Admit(ctx context.Context, source netip.Addr, subject string) error {
	source = source.Unmap()
	if a == nil || a.store == nil || a.limits.Validate() != nil || !identity.ValidID(subject) || !source.IsValid() || source.Zone() != "" || source.IsUnspecified() || source.IsMulticast() {
		return ErrInvalid
	}
	keys := []string{a.prefix + "config", a.prefix + a.mac("global", ""), a.prefix + a.mac("source", source.String()), a.prefix + a.mac("subject", subject)}
	args := []any{a.metadata}
	for _, b := range a.limits.buckets() {
		args = append(args, b.Count, b.Window.Milliseconds())
	}
	allowed, err := runAdmission(ctx, a.store, passwordChangeAdmitLua, keys, args)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPasswordChangeRate
	}
	return nil
}

package redisstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
)

// ErrEmailRate identifies a denial without revealing which quota was exhausted.
var ErrEmailRate = errors.New("email request admission rate exceeded")

var emailAdmitLua = strings.Replace(loginAdmitLua, "-- session-protocol:login-admit", "-- session-protocol:email-admit", 1)

// EmailLimits defines separate request and confirmation quotas. Within each
// group, activation and recovery share the global and source budgets.
type EmailLimits struct {
	RequestGlobal, RequestSource, RequestMailbox LoginBucket
	ConfirmGlobal, ConfirmSource, ConfirmToken   LoginBucket
}

func (c EmailLimits) buckets(confirm bool) [3]LoginBucket {
	if confirm {
		return [3]LoginBucket{c.ConfirmGlobal, c.ConfirmSource, c.ConfirmToken}
	}
	return [3]LoginBucket{c.RequestGlobal, c.RequestSource, c.RequestMailbox}
}

// Validate checks the explicit bounds without reading Redis.
func (c EmailLimits) Validate() error {
	for _, group := range [2][3]LoginBucket{c.buckets(false), c.buckets(true)} {
		for _, bucket := range group {
			if !bucket.valid() {
				return ErrInvalid
			}
		}
	}
	return nil
}

// EmailAdmission shares separate request and confirmation quotas across API replicas without
// placing email addresses, token digests, or source addresses in Redis keys.
type EmailAdmission struct {
	store            *Store
	limits           EmailLimits
	key              [32]byte
	prefix, metadata string
}

// Format keeps the limiter key out of direct diagnostics.
func (EmailAdmission) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte("[redacted email admission]"))
}

func newEmailAdmission(store *Store, secrets *LoginSecrets, limits EmailLimits) (*EmailAdmission, error) {
	if store == nil || !boundedClient(store.client) || !namespacePattern.MatchString(store.namespace) || limits.Validate() != nil {
		return nil, ErrInvalid
	}
	if !validLoginSecrets(secrets) {
		return nil, ErrLoginSecrets
	}
	a := &EmailAdmission{store: store, limits: limits, key: secrets.limiting, prefix: "oj-email-limit:" + store.namespace + ":"}
	var config strings.Builder
	for _, group := range [2][3]LoginBucket{limits.buckets(false), limits.buckets(true)} {
		for _, bucket := range group {
			_, _ = fmt.Fprintf(&config, "%d/%d;", bucket.Count, bucket.Window.Milliseconds())
		}
	}
	a.metadata = "1:" + a.mac("config", config.String())
	return a, nil
}

func (a *EmailAdmission) mac(label, value string) string {
	m := hmac.New(sha256.New, a.key[:])
	_, _ = m.Write([]byte("oj-email-limit-v1\x00" + a.store.namespace + "\x00" + label + "\x00" + value))
	return hex.EncodeToString(m.Sum(nil))
}

// InitializeEmailLimits provisions the marker only on a known initial
// namespace under deployment isolation. This is not startup repair.
func InitializeEmailLimits(ctx context.Context, store *Store, secrets *LoginSecrets, limits EmailLimits) error {
	a, err := newEmailAdmission(store, secrets, limits)
	if err != nil {
		return err
	}
	return a.checkMarker(ctx, "#!lua flags=no-cluster\n-- session-protocol:email-limit-init\n"+
		"if redis.call('TYPE', KEYS[1]).ok == 'none' then redis.call('SET', KEYS[1], ARGV[1], 'NX') end\n")
}

// NewEmailAdmission refuses missing or incompatible markers without creating
// them. All replicas must use the same limits, namespace and key.
func NewEmailAdmission(ctx context.Context, store *Store, secrets *LoginSecrets, limits EmailLimits) (*EmailAdmission, error) {
	a, err := newEmailAdmission(store, secrets, limits)
	if err != nil {
		return nil, err
	}
	if err := a.checkMarker(ctx, "#!lua flags=no-writes,no-cluster\n-- session-protocol:email-limit-check\n"); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *EmailAdmission) checkMarker(ctx context.Context, script string) error {
	allowed, err := runAdmission(ctx, a.store, script+loginLimitsMetadataLua, []string{a.prefix + "config"}, []any{a.metadata})
	if err == nil && !allowed {
		return storageError{ErrConflict}
	}
	return err
}

// AdmitRequest charges before account lookup, including for unknown addresses.
// A public response must not expose a mailbox-specific quota outcome.
func (a *EmailAdmission) AdmitRequest(ctx context.Context, source netip.Addr, email identity.Email) error {
	if !email.Valid() {
		return ErrInvalid
	}
	return a.admit(ctx, source, "request", email.Key(), false)
}

// AdmitConfirmation charges before password calculation. The digest was
// computed by the application from the canonical token and its fixed purpose.
func (a *EmailAdmission) AdmitConfirmation(ctx context.Context, source netip.Addr, purpose emailtoken.Purpose, digest [32]byte) error {
	if purpose.Lifetime() == 0 || digest == ([32]byte{}) {
		return ErrInvalid
	}
	return a.admit(ctx, source, "confirm", string(purpose)+"\x00"+hex.EncodeToString(digest[:]), true)
}

func (a *EmailAdmission) admit(ctx context.Context, source netip.Addr, group, target string, confirm bool) error {
	source = source.Unmap()
	if a == nil || a.store == nil || !source.IsValid() || source.Zone() != "" || source.IsUnspecified() || source.IsMulticast() {
		return ErrInvalid
	}
	keys := []string{a.prefix + "config", a.prefix + a.mac(group+"-global", ""),
		a.prefix + a.mac(group+"-source", source.String()), a.prefix + a.mac(group+"-target", target)}
	args := []any{a.metadata}
	for _, bucket := range a.limits.buckets(confirm) {
		args = append(args, bucket.Count, bucket.Window.Milliseconds())
	}
	allowed, err := runAdmission(ctx, a.store, emailAdmitLua, keys, args)
	if err == nil && !allowed {
		return ErrEmailRate
	}
	return err
}

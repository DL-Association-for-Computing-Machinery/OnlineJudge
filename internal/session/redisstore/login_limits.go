package redisstore

import (
	"context"
	_ "embed"
	"net/netip"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

//go:embed lua/login_admit.lua
var loginAdmitLua string

const loginLimitsMetadataLua = `
local kind = redis.call('TYPE', KEYS[1]).ok
if kind ~= 'string' or redis.call('GET', KEYS[1]) ~= ARGV[1] or
   redis.call('PEXPIRETIME', KEYS[1]) ~= -1 then return -1 end
return 1
`

// InitializeLoginLimits is an explicit provisioning operation, never called by
// NewLimitedLogin or request admission. Use only under deployment isolation on a
// known initial namespace. It neither proves Redis continuity nor repairs lost
// budgets: after data loss, quiesce all replicas and follow restore procedures.
// An identical existing persistent marker is idempotent; mismatch fails closed.
func InitializeLoginLimits(ctx context.Context, store *Store, secrets *LoginSecrets, limits LoginLimits) error {
	a, err := newLoginAdmission(store, secrets, limits)
	if err != nil {
		return err
	}
	script := "#!lua flags=no-cluster\n-- session-protocol:login-limit-init\n" + `
if redis.call('TYPE', KEYS[1]).ok == 'none' then
    redis.call('SET', KEYS[1], ARGV[1], 'NX')
end
` + loginLimitsMetadataLua
	return a.run(ctx, script, []string{a.prefix + "config"}, []any{a.metadata})
}

// NewLimitedLogin binds admission to the same Login/Store and loaded secrets.
// The persistent configuration marker must already exist and match; construction
// only reads it and will never recreate a missing marker or reset counters.
func NewLimitedLogin(ctx context.Context, store *Store, origin string, window time.Duration, secrets *LoginSecrets, limits LoginLimits) (*Login, error) {
	l, err := NewLoginFromSecrets(store, origin, window, secrets)
	if err != nil {
		return nil, err
	}
	a, err := newLoginAdmission(store, secrets, limits)
	if err != nil {
		return nil, err
	}
	if err = a.run(ctx, "#!lua flags=no-writes,no-cluster\n-- session-protocol:login-limit-check\n"+loginLimitsMetadataLua, []string{a.prefix + "config"}, []any{a.metadata}); err != nil {
		return nil, err
	}
	l.admission = a
	return l, nil
}

// RateLimited reports whether this manager carries an immutable admission owner.
// Core Prepare/Complete/Abandon do not infer a network source; transports must
// call the matching admission method before identity/session I/O or password work.
func (l *Login) RateLimited() bool { return l != nil && l.admission != nil }

// AdmitBegin atomically charges global, source, and canonical email budgets.
// Every admitted request counts regardless of credential outcome. A denial does
// not write any dimension. Unknown/partial effects are never retried or refunded.
func (l *Login) AdmitBegin(ctx context.Context, source netip.Addr, email identity.Email) error {
	if !l.RateLimited() || !email.Valid() {
		return ErrInvalid
	}
	return l.admission.admit(ctx, source, email.Key(), false)
}

// AdmitRecovery shares one global/source budget across complete and abandon,
// independent of begin. Quota denial leaves the original candidate/cookies alone.
func (l *Login) AdmitRecovery(ctx context.Context, source netip.Addr) error {
	if !l.RateLimited() {
		return ErrInvalid
	}
	return l.admission.admit(ctx, source, "", true)
}

func (a *loginAdmission) admit(ctx context.Context, source netip.Addr, email string, recovery bool) error {
	if !source.IsValid() || source.Zone() != "" {
		return ErrInvalid
	}
	op := "begin"
	if recovery {
		op = "recovery"
	}
	keys := []string{a.prefix + "config", a.prefix + a.mac(op+"-global", ""), a.prefix + a.mac(op+"-source", source.Unmap().String())}
	if !recovery {
		keys = append(keys, a.prefix+a.mac(op+"-email", email))
	}
	args := []any{a.metadata}
	for _, b := range a.limits.buckets(recovery) {
		args = append(args, b.Count, b.Window.Milliseconds())
	}
	return a.run(ctx, loginAdmitLua, keys, args)
}

func (a *loginAdmission) run(ctx context.Context, script string, keys []string, args []any) error {
	allowed, err := runAdmission(ctx, a.store, script, keys, args)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrLoginRate
	}
	return nil
}

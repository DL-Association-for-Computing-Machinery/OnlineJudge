package redisstore

import (
	"context"
	"crypto/cipher"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// LoginConfig requires explicitly provisioned shared sealing keys. All replicas
// must share Origin, Window, namespace and policy. Never generate keys at startup.
// Operators own key rotation and the aggregate GCM limit of 2^32 seals per key.
type LoginConfig struct {
	Origin     string
	Window     time.Duration
	CurrentKey string
	Keys       map[string][32]byte
}

func (c LoginConfig) String() string   { return "[redacted login sealing configuration]" }
func (c LoginConfig) GoString() string { return c.String() }

// Login owns a single session store and immutable authenticated-encryption setup.
// It has no pending-attempt map, password persistence or separate session backend.
type Login struct {
	store   *Store
	origin  string
	window  time.Duration
	current string
	keys    map[string]cipher.AEAD
}

func (l *Login) String() string   { return "[redacted login manager]" }
func (l *Login) GoString() string { return l.String() }

// Continuation contains an encrypted frozen candidate, never an authenticated
// session. Value is only for the HttpOnly attempt cookie; never log it.
type Continuation struct {
	value string
	until time.Time
}

func (c Continuation) Value() string    { return c.value }
func (c Continuation) String() string   { return "[redacted login continuation]" }
func (c Continuation) GoString() string { return c.String() }

// Deadline is populated only by successful Prepare, not by parsing browser data.
func (c Continuation) Deadline() time.Time { return c.until }

// Origin returns the public origin bound into every continuation.
func (l *Login) Origin() string {
	if l == nil {
		return ""
	}
	return l.origin
}

// Prepare verifies credentials and freezes a candidate without a session write.
// A dropped response therefore consumes no session slot. Any gate/seal failure
// returns no continuation, although a password upgrade may already have committed.
func (l *Login) Prepare(ctx context.Context, email identity.Email, secret []byte) (Continuation, error) {
	if l == nil || l.store == nil {
		return Continuation{}, ErrInvalid
	}
	a, err := l.store.prepare(ctx, email, secret, l.window, false)
	if err != nil {
		return Continuation{}, err
	}
	if err = ctx.Err(); err != nil {
		return Continuation{}, err
	}
	c, err := l.seal(a)
	if err != nil {
		return Continuation{}, err
	}
	if err = ctx.Err(); err != nil {
		return Continuation{}, err
	}
	return c, nil
}

// Complete replays only the sealed tuple, then separately checks the full receipt
// and current authorities. An error never publishes a credential. Concurrent or
// repeated calls can return the same token, not an exactly-once HTTP response.
func (l *Login) Complete(ctx context.Context, c Continuation) (Credential, error) {
	a, err := l.open(c)
	if err != nil {
		return Credential{}, err
	}
	if err = l.store.Retry(ctx, a); err != nil {
		return Credential{}, err
	}
	return l.store.Check(ctx, a)
}

// Abandon tombstones only this candidate, including after its login window.
// It neither clears a browser session cookie nor proves an earlier write rolled
// back. Original absolute expiry and both authority gates still apply.
func (l *Login) Abandon(ctx context.Context, c Continuation) error {
	a, err := l.open(c)
	if err != nil {
		return err
	}
	return l.store.Abandon(ctx, a)
}

// Read delegates to the same owner used for login; it never renews a session.
func (l *Login) Read(ctx context.Context, c Credential) (View, error) {
	if l == nil || l.store == nil {
		return View{}, ErrInvalid
	}
	return l.store.Read(ctx, c)
}

// Touch delegates explicit activity to the same session owner.
func (l *Login) Touch(ctx context.Context, c Credential) (View, error) {
	if l == nil || l.store == nil {
		return View{}, ErrInvalid
	}
	return l.store.Touch(ctx, c)
}

// Revoke delegates confirmed logout to the same session owner.
func (l *Login) Revoke(ctx context.Context, c Credential) error {
	if l == nil || l.store == nil {
		return ErrInvalid
	}
	return l.store.Revoke(ctx, c)
}

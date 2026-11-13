// Package emailconfirm applies password and admission policy before an atomic
// email activation or password reset.
package emailconfirm

import (
	"context"
	"errors"
	"net/netip"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

// ErrLink is the uniform result for unavailable, expired, or consumed links.
var ErrLink = errors.New("account email link unavailable")

// Store owns the final atomic token and credential checks. An error may follow
// a committed write; callers must not retry automatically. Implementations
// must not retain the supplied record slices after returning.
type Store interface {
	ActivateEmail(context.Context, emailtoken.Token, password.Record) error
	ResetPasswordByEmail(context.Context, emailtoken.Token, password.Record) error
}

// Admit charges shared request quotas before any password calculation. The
// address is transport-observed, never taken from request data. The digest is
// purpose-specific and should be keyed again before storage as a quota key.
type Admit func(context.Context, netip.Addr, emailtoken.Purpose, [32]byte) error

// Confirmer uses an existing process-wide password work budget. It neither
// issues a session nor exposes an HTTP endpoint.
type Confirmer struct {
	store Store
	work  *password.BoundedHasher
	rules *password.Acceptance
	admit Admit
}

// New requires compatible password rules and a confirmation admission owner.
func New(store Store, work *password.BoundedHasher, rules *password.Acceptance, admit Admit) (*Confirmer, error) {
	if store == nil || admit == nil || work.MaxPasswordBytes() == 0 ||
		rules.Fingerprint() == ([32]byte{}) || rules.MaxBytes() != work.MaxPasswordBytes() ||
		rules.MinimumCharacters() < password.MinimumNewPasswordCharacters {
		return nil, identity.ErrInvalid
	}
	return &Confirmer{store: store, work: work, rules: rules, admit: admit}, nil
}

// Activate verifies the activation link and sets the mailbox owner's password.
func (c *Confirmer) Activate(ctx context.Context, source netip.Addr, encodedToken string, replacement []byte) error {
	return c.confirm(ctx, source, emailtoken.Activation, encodedToken, replacement)
}

// Reset replaces a password using a recovery link for a verified mailbox.
func (c *Confirmer) Reset(ctx context.Context, source netip.Addr, encodedToken string, replacement []byte) error {
	return c.confirm(ctx, source, emailtoken.Recovery, encodedToken, replacement)
}

func (c *Confirmer) confirm(ctx context.Context, source netip.Addr, purpose emailtoken.Purpose, encoded string, replacement []byte) error {
	source = source.Unmap()
	if c == nil || c.admit == nil || ctx == nil || !source.IsValid() || source.Zone() != "" ||
		source.IsUnspecified() || source.IsMulticast() {
		return identity.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	token, err := emailtoken.Parse(encoded)
	if err != nil {
		return ErrLink
	}
	digest, err := token.Digest(purpose)
	if err != nil {
		return ErrLink
	}
	if err := c.admit(ctx, source, purpose, digest); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.rules.Check(replacement); err != nil {
		return err
	}
	record, err := c.work.Create(ctx, replacement)
	if err != nil {
		return err
	}
	defer clear(record.Salt)
	defer clear(record.Digest)
	if err := ctx.Err(); err != nil {
		return err
	}
	if purpose == emailtoken.Activation {
		err = c.store.ActivateEmail(ctx, token, record)
	} else {
		err = c.store.ResetPasswordByEmail(ctx, token, record)
	}
	if errors.Is(err, identity.ErrConflict) {
		return ErrLink
	}
	return err
}

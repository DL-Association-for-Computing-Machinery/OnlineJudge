package identity

import (
	"context"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

// PersonalCredentialStore owns all facts used by a personal credential workflow.
// Every method must use the same authoritative database and subject namespace,
// not per-method caches, replicas or tenants. One interface cannot prove that.
// WithSessionCredentials must lock and recheck global recovery then account
// authority in one transaction, with the existing uncertain-callback contract.
type PersonalCredentialStore interface {
	CredentialStore
	RecoveryState(context.Context) (RecoveryState, error)
	LookupPersonalByEmail(context.Context, Email) (Personal, error)
	WithSessionCredentials(context.Context, string, Revision, string, func(context.Context, SessionState) error) error
}

// PersonalCredentialGate connects email/password verification to session-storage
// authority. It does not issue sessions, activate email or grant resource rights.
type PersonalCredentialGate struct {
	store    PersonalCredentialStore
	verifier *CredentialVerifier
}

// NewPersonalCredentialGate constructs verification using the same store owner.
// All password flows must share work and use compatible approved store policies.
// Neither this constructor nor the workflow configures production budgets.
func NewPersonalCredentialGate(store PersonalCredentialStore, work *password.BoundedHasher) (*PersonalCredentialGate, error) {
	verifier, err := NewCredentialVerifier(store, work)
	if err != nil {
		return nil, err
	}
	return &PersonalCredentialGate{store: store, verifier: verifier}, nil
}

// WithPersonalCredentials captures an open recovery epoch before email lookup
// and password work, verifies the resolved subject, then passes that exact epoch
// and the verification's revisions to the final locked authority check.
// It never rereads an epoch to relabel old work or retries a failed upgrade/gate.
//
// operation is a trusted synchronous session-storage callback, at most once,
// subject to WithSessionCredentials' cancellation and uncertain-effect contract.
// Do not publish HTTP/Cookies or run KDF in it. Errors after it starts may carry
// ErrGuardedOperation; preserve that before interpreting underlying causes.
// The caller owns the whole-call deadline and keeps secret unchanged until return.
// EmailVerified is not a credential or training grant; permissions remain separate.
func (g *PersonalCredentialGate) WithPersonalCredentials(ctx context.Context, email Email, secret []byte, operation func(context.Context, SessionState) error) error {
	if g == nil || g.store == nil || g.verifier == nil || ctx == nil || !email.Valid() || operation == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	recovery, err := g.store.RecoveryState(ctx)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if (recovery.Generation != "" && !ValidRecoveryID(recovery.Generation)) || (recovery.Enabled && recovery.Generation == "") {
		return ErrStore
	}
	if !recovery.Enabled {
		return ErrRecovering
	}
	personal, err := g.store.LookupPersonalByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrCredentials
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !ValidID(personal.ID) || !personal.Valid() || !personal.Email.Valid() || personal.Email.Key() != email.Key() {
		return ErrStore
	}
	verified, err := g.verifier.VerifyPassword(ctx, personal.ID, secret)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Mapping revisions are only a lookup observation. Legacy upgrading may
	// have changed credential version; the verifier's exact revision is required.
	return g.store.WithSessionCredentials(ctx, personal.ID, verified.Revision, recovery.Generation, operation)
}

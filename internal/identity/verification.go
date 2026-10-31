package identity

import (
	"context"
	"errors"
	"math"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

// ErrCredentials reports an absent/disabled subject or a password mismatch.
// It does not promise indistinguishable timing for a future network endpoint.
var ErrCredentials = errors.New("credentials not accepted")

// CredentialStore is the identity-owned boundary used for password verification.
// Reads are point-in-time observations. RehashCAS must atomically require an
// enabled subject and both expected revisions, incrementing only credential
// version. Errors must preserve uncertain writes and redact password material.
type CredentialStore interface {
	Get(context.Context, string) (Snapshot, error)
	State(context.Context, string) (State, error)
	RehashCAS(context.Context, string, Revision, password.Record) (Snapshot, error)
}

type passwordWork interface {
	Create(context.Context, []byte) (password.Record, error)
	Verify(context.Context, []byte, password.Record) (password.Verification, error)
}

// CredentialVerification contains no password material. It describes the last
// successful State read, not a reusable proof, session grant, or authorization.
// On failure the zero result does not mean a prior upgrade was not committed.
type CredentialVerification struct {
	State
	Upgraded bool
}

// CredentialVerifier connects bounded password work to conditional persistence.
// Its zero value fails closed. All flows must share the same BoundedHasher;
// the store and hasher must use compatible approved password policies.
type CredentialVerifier struct {
	store CredentialStore
	work  passwordWork
}

// NewCredentialVerifier uses a caller-owned store and initialized BoundedHasher.
// It does not resolve email identifiers, configure budgets, or issue sessions.
func NewCredentialVerifier(store CredentialStore, work *password.BoundedHasher) (*CredentialVerifier, error) {
	if store == nil || work == nil {
		return nil, ErrInvalid
	}
	return &CredentialVerifier{store: store, work: work}, nil
}

// VerifyPassword checks an already resolved opaque subject ID, upgrades a
// matching Legacy record using the same password, and rechecks exact state.
// Callers must not modify secret until this whole call returns. Each Verify and
// Create separately acquires the shared budget, outside all database locks.
// Failed upgrades fail this call without retries or compensating writes. A
// later session workflow still requires its own atomic authority guard.
func (v *CredentialVerifier) VerifyPassword(ctx context.Context, id string, secret []byte) (CredentialVerification, error) {
	if v == nil || v.store == nil || v.work == nil || ctx == nil || !ValidID(id) {
		return CredentialVerification{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return CredentialVerification{}, err
	}
	snapshot, err := v.store.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return CredentialVerification{}, ErrCredentials
		}
		return CredentialVerification{}, err
	}
	if err := ctx.Err(); err != nil {
		return CredentialVerification{}, err
	}
	if snapshot.ID != id || !snapshot.Valid() {
		return CredentialVerification{}, ErrStore
	}
	if !snapshot.Enabled {
		return CredentialVerification{}, ErrCredentials
	}
	match, err := v.work.Verify(ctx, secret, snapshot.Record)
	if err != nil {
		return CredentialVerification{}, err
	}
	if err := ctx.Err(); err != nil {
		return CredentialVerification{}, err
	}
	if !match.Match {
		return CredentialVerification{}, ErrCredentials
	}
	want := snapshot.State
	if match.NeedsUpgrade {
		if want.CredentialVersion == math.MaxInt64 {
			return CredentialVerification{}, ErrConflict
		}
		record, err := v.work.Create(ctx, secret)
		if err != nil {
			return CredentialVerification{}, err
		}
		if err := ctx.Err(); err != nil {
			return CredentialVerification{}, err
		}
		upgraded, err := v.store.RehashCAS(ctx, id, want.Revision, record)
		if err != nil {
			// A canceled write may already have committed. Never replace its
			// storage classification with ctx.Err or retry from a fresh read.
			return CredentialVerification{}, err
		}
		if err := ctx.Err(); err != nil {
			return CredentialVerification{}, err
		}
		want.CredentialVersion++
		if upgraded.State != want {
			return CredentialVerification{}, ErrStore
		}
	}
	state, err := v.store.State(ctx, id)
	if err != nil {
		return CredentialVerification{}, err
	}
	if err := ctx.Err(); err != nil {
		return CredentialVerification{}, err
	}
	if state != want {
		return CredentialVerification{}, ErrConflict
	}
	return CredentialVerification{State: state, Upgraded: match.NeedsUpgrade}, nil
}

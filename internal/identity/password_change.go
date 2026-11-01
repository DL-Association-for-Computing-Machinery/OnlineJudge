package identity

import (
	"bytes"
	"context"
	"errors"
	"math"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

// ErrPasswordUnchanged rejects a byte-identical replacement after current
// password verification. No normalization or product strength policy is applied.
var ErrPasswordUnchanged = errors.New("replacement password must differ")

// PasswordChangeStore owns the single authority used by authenticated changes.
// All methods must refer to the same database and subject namespace. The final
// replacement must lock open recovery at the captured epoch, require enabled
// state and both revisions, and advance both revisions atomically. An error may
// follow a committed write and never permits an automatic retry.
type PasswordChangeStore interface {
	Get(context.Context, string) (Snapshot, error)
	RecoveryState(context.Context) (RecoveryState, error)
	ReplacePasswordForSession(context.Context, string, Revision, string, password.Record) (Snapshot, error)
}

// PasswordChanger verifies and replaces credentials against an existing-session
// fence. The zero value fails closed; this does not validate a session's Redis
// lifetime/slot, grant resource rights or choose new-password product rules.
type PasswordChanger struct {
	store PasswordChangeStore
	work  passwordWork
}

// NewPasswordChanger binds one store owner and the shared password admission
// budget. Store and work must use compatible approved password policies.
func NewPasswordChanger(store PasswordChangeStore, work *password.BoundedHasher) (*PasswordChanger, error) {
	if store == nil || work == nil {
		return nil, ErrInvalid
	}
	return &PasswordChanger{store: store, work: work}, nil
}

// Change uses the version from the caller's first real session Read. Never
// replace that fence with a fresh account/recovery read. Current verification
// and replacement creation run outside database locks; Legacy verification
// does not first rehash. The final replacement is attempted exactly once.
// Callers retain both password slices unchanged until return. Any error returns
// zero State, including cancellation or unknown outcome after a committed write.
func (c *PasswordChanger) Change(ctx context.Context, id string, version SessionVersion, current, replacement []byte) (State, error) {
	if c == nil || c.store == nil || c.work == nil || ctx == nil || !ValidID(id) || !version.Valid() {
		return State{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	recovery, err := c.store.RecoveryState(ctx)
	if err != nil {
		return State{}, err
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	if (recovery.Generation != "" && !ValidRecoveryID(recovery.Generation)) || (recovery.Enabled && recovery.Generation == "") {
		return State{}, ErrStore
	}
	if !recovery.Enabled {
		return State{}, ErrRecovering
	}
	if recovery.Generation != version.RecoveryID {
		return State{}, ErrConflict
	}
	snapshot, err := c.store.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return State{}, ErrCredentials
		}
		return State{}, err
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	if snapshot.ID != id || !snapshot.Valid() {
		return State{}, ErrStore
	}
	if !snapshot.Enabled || snapshot.AuthGeneration != version.AuthGeneration ||
		snapshot.CredentialVersion == math.MaxInt64 || snapshot.AuthGeneration == math.MaxInt64 {
		return State{}, ErrConflict
	}
	match, err := c.work.Verify(ctx, current, snapshot.Record)
	if err != nil {
		return State{}, err
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	if !match.Match {
		return State{}, ErrCredentials
	}
	if bytes.Equal(current, replacement) {
		return State{}, ErrPasswordUnchanged
	}
	record, err := c.work.Create(ctx, replacement)
	if err != nil {
		return State{}, err
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	changed, err := c.store.ReplacePasswordForSession(ctx, id, snapshot.Revision, version.RecoveryID, record)
	if err != nil {
		// Preserve storage/unknown-write classification even if cancellation
		// became visible during persistence. Never reread, retry or compensate.
		return State{}, err
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	want := snapshot.State
	want.CredentialVersion++
	want.AuthGeneration++
	if changed.State != want || !samePasswordRecord(changed.Record, record) {
		return State{}, ErrStore
	}
	return changed.State, nil
}

func samePasswordRecord(a, b password.Record) bool {
	return a.FormatVersion == b.FormatVersion && a.Algorithm == b.Algorithm && a.ArgonVersion == b.ArgonVersion &&
		a.Parameters == b.Parameters && bytes.Equal(a.Salt, b.Salt) && bytes.Equal(a.Digest, b.Digest)
}

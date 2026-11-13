package redisstore

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

// MinimumNewPasswordCharacters is the initial network password-change floor.
// It follows the single-factor length recommendation; it does not claim full
// NIST compliance or prescribe dictionary coverage/KDF production parameters.
const MinimumNewPasswordCharacters = password.MinimumNewPasswordCharacters

// PasswordChange binds session authentication, independent quotas, new-password
// rules and credential replacement to one Login/store/authority/shared KDF owner.
// It has no public subject selector or replaceable authorization callback.
type PasswordChange struct {
	login      *Login
	admission  *PasswordChangeAdmission
	acceptance *password.Acceptance
	changer    *identity.PasswordChanger
}

func (PasswordChange) String() string                   { return "[redacted password change owner]" }
func (c PasswordChange) GoString() string               { return c.String() }
func (c PasswordChange) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(c.String())) }

// NewPasswordChange uses exactly the authority and password budget retained by
// the login's original Store. Construction only checks existing quota metadata,
// never provisions it or repairs state. Rules must use that KDF byte budget.
func NewPasswordChange(ctx context.Context, login *Login, secrets *LoginSecrets, limits PasswordChangeLimits, acceptance *password.Acceptance) (*PasswordChange, error) {
	if !login.RateLimited() || login.store == nil || acceptance.Fingerprint() == ([32]byte{}) || acceptance.MinimumCharacters() < MinimumNewPasswordCharacters || acceptance.MaxBytes() != login.store.work.MaxPasswordBytes() {
		return nil, ErrInvalid
	}
	authority, ok := login.store.authority.(identity.PasswordChangeStore)
	if !ok {
		return nil, ErrInvalid
	}
	changer, err := identity.NewPasswordChanger(authority, login.store.work)
	if err != nil {
		return nil, err
	}
	admission, err := NewPasswordChangeAdmission(ctx, login.store, secrets, limits, acceptance)
	if err != nil {
		return nil, err
	}
	return &PasswordChange{login: login, admission: admission, acceptance: acceptance, changer: changer}, nil
}

// ForLogin checks immutable owner association for the transport constructor.
func (c *PasswordChange) ForLogin(login *Login) bool {
	return c != nil && c.login != nil && c.login == login && c.changer != nil && c.admission != nil && c.acceptance.Fingerprint() != ([32]byte{})
}

// Change authenticates once without renewal and freezes that exact session's
// account/recovery generations. It never substitutes later authority into the
// fence. Logout/expiry after this Read do not undo already-admitted work; final
// account/recovery CAS, current-password verification and all quotas still apply.
// Success invalidates old generations through PostgreSQL, without depending on
// Redis cleanup. Any error may follow a committed change: never retry blindly.
func (c *PasswordChange) Change(ctx context.Context, credential Credential, source netip.Addr, current, replacement []byte) error {
	if c == nil || !c.ForLogin(c.login) || ctx == nil {
		return ErrInvalid
	}
	view, err := c.login.Read(ctx, credential)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	version := identity.SessionVersion{AuthGeneration: view.AuthGeneration, RecoveryID: view.RecoveryID}
	if !identity.ValidID(view.SubjectID) || !version.Valid() {
		return storageError{ErrConflict}
	}
	if err := c.admission.Admit(ctx, source, view.SubjectID); err != nil {
		return err
	}
	if err := c.acceptance.Check(replacement); err != nil {
		return err
	}
	_, err = c.changer.Change(ctx, view.SubjectID, version, current, replacement)
	return err
}

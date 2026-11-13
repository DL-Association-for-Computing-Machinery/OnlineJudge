package postgres

import (
	"context"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

// ProcessEmailRequest handles one admitted background request. The caller must
// capture the recovery generation at intake and keep this result off the public
// response path. Nil may mean no mail. An error may follow a committed write;
// the caller must not automatically repeat the request.
func (s *Store) ProcessEmailRequest(ctx context.Context, email identity.Email, purpose emailtoken.Purpose, recoveryID string,
	work *password.BoundedHasher, protector *emailtoken.Protector, reserve EmailTokenSealBudget, enqueue EmailTokenEnqueue,
) error {
	if err := s.checkPersonal(ctx); err != nil {
		return err
	}
	if !email.Valid() || !identity.ValidRecoveryID(recoveryID) || purpose.Lifetime() == 0 ||
		work.MaxPasswordBytes() < 32 || protector.CurrentKeyID() == "" || reserve == nil || enqueue == nil {
		return identity.ErrInvalid
	}
	state, err := s.RecoveryState(ctx)
	if err != nil {
		return err
	}
	if !state.Enabled || state.Generation != recoveryID {
		return nil
	}

	account, err := s.LookupPersonalByEmail(ctx, email)
	if errors.Is(err, identity.ErrNotFound) {
		if purpose != emailtoken.Activation {
			return nil
		}
		// A competing request may have registered this mailbox since the read.
		// Its first email is sufficient; don't fall through to a second write.
		return emailRequestResult(s.RegisterPersonal(ctx, email, recoveryID, work, protector, reserve, enqueue))
	}
	if err != nil {
		return err
	}
	if !account.Enabled || account.EmailVerified != (purpose == emailtoken.Recovery) {
		return nil
	}
	version := identity.SessionVersion{AuthGeneration: account.AuthGeneration, RecoveryID: recoveryID}
	_, err = s.IssueEmailToken(ctx, account.ID, version, purpose, protector, reserve, enqueue)
	return emailRequestResult(err)
}

func emailRequestResult(err error) error {
	// A nested driver or callback error can carry its own cause. Preserve an
	// uncertain storage result before classifying definitive business denials.
	if !errors.Is(err, identity.ErrStore) &&
		(errors.Is(err, identity.ErrConflict) || errors.Is(err, identity.ErrRecovering)) {
		return nil
	}
	return err
}

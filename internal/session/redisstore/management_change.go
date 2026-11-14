package redisstore

import (
	"context"
	"errors"
	"net/netip"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// ManagementRoleWriter owns transactional role changes, including actor and
// target checks. It never verifies a password or owns Redis.
type ManagementRoleWriter interface {
	ChangeAsAdmin(context.Context, identity.PlatformAdminActor, int64, identity.PlatformAdminRoleRequest, func(context.Context) error) (identity.PlatformAdminRole, error)
}

// ManagementRoleChanges performs fresh password verification for each write.
// It has no standalone reauthentication endpoint or reusable grant.
type ManagementRoleChanges struct {
	sessions *Management
	login    *Login
	writer   ManagementRoleWriter
	personal managementPersonalReader
}

// NewManagementRoleChanges reuses the same bounded ordinary store and limiter.
func NewManagementRoleChanges(sessions *Management, login *Login, writer ManagementRoleWriter) (*ManagementRoleChanges, error) {
	if sessions == nil || sessions.store == nil || login == nil || login.store != sessions.store || !login.RateLimited() || writer == nil {
		return nil, ErrInvalid
	}
	personal, ok := sessions.store.authority.(managementPersonalReader)
	if !ok {
		return nil, ErrInvalid
	}
	return &ManagementRoleChanges{sessions: sessions, login: login, writer: writer, personal: personal}, nil
}

// Change authenticates the current slot and password, then appends one role
// revision if the same slot is still usable at the write's authority check.
// The request neither touches the slot nor changes the browser's credential.
func (s *ManagementRoleChanges) Change(ctx context.Context, credential Credential, source netip.Addr, password []byte, request identity.PlatformAdminRoleRequest) (identity.PlatformAdminRole, error) {
	var zero identity.PlatformAdminRole
	if s == nil || ctx == nil || !source.IsValid() || !request.Valid() || len(password) == 0 {
		return zero, ErrInvalid
	}
	view, err := s.sessions.Read(ctx, credential)
	if err != nil {
		if errors.Is(err, identity.ErrConflict) && !errors.Is(err, identity.ErrGuardedOperation) && !errors.Is(err, identity.ErrStore) {
			return zero, identity.ErrPlatformAdminRequired
		}
		return zero, err
	}
	actor := view.Actor
	if request.SubjectID == actor.SubjectID {
		return zero, identity.ErrInvalid
	}
	store := s.sessions.store
	// Resolve the limiter's email from the actor's subject, not request data.
	if err := s.admit(ctx, actor, source); err != nil {
		return zero, err
	}
	snapshot, err := store.authority.Get(ctx, actor.SubjectID)
	if errors.Is(err, identity.ErrNotFound) {
		return zero, identity.ErrPlatformAdminRequired
	}
	if err != nil {
		return zero, err
	}
	if snapshot.ID != actor.SubjectID || !snapshot.Valid() || !snapshot.Enabled || snapshot.AuthGeneration != actor.AuthGeneration {
		return zero, identity.ErrPlatformAdminRequired
	}
	verified, err := store.work.Verify(ctx, password, snapshot.Record)
	if err != nil {
		return zero, err
	}
	if !verified.Match {
		return zero, identity.ErrCredentials
	}
	return s.writer.ChangeAsAdmin(ctx, actor, snapshot.CredentialVersion, request, func(call context.Context) error {
		// Read only Redis while the writer owns PostgreSQL actor/target locks.
		// Activity may change the idle deadline. The original actor and absolute
		// deadline must match even when a concurrent activity renewed the slot.
		current, err := s.sessions.run(call, "locate", credential, "")
		if err != nil {
			return err
		}
		if current.actor != actor || current.absolute != view.AbsoluteDeadline.UnixMilli() {
			return identity.ErrPlatformAdminRequired
		}
		return nil
	})
}

type managementPersonalReader interface {
	PersonalByID(context.Context, string) (identity.Personal, error)
}

func (s *ManagementRoleChanges) admit(ctx context.Context, actor identity.PlatformAdminActor, source netip.Addr) error {
	personal, err := s.personal.PersonalByID(ctx, actor.SubjectID)
	if errors.Is(err, identity.ErrNotFound) {
		return identity.ErrPlatformAdminRequired
	}
	if err != nil {
		return err
	}
	if !personal.Enabled || !personal.EmailVerified || personal.ID != actor.SubjectID || personal.AuthGeneration != actor.AuthGeneration {
		return identity.ErrPlatformAdminRequired
	}
	return s.login.AdmitBegin(ctx, source, personal.Email)
}

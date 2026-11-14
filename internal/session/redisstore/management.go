package redisstore

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

const managementMaxRecord = 224

var managementPolicy = Policy{Idle: 30 * time.Minute, Absolute: 8 * time.Hour}

// ManagementAuthority coordinates the platform role and exact account snapshot.
type ManagementAuthority interface {
	Inspect(context.Context, string) (identity.PlatformAdminRole, error)
	WithAdminAdmission(context.Context, identity.SessionState, int64, func(context.Context, identity.PlatformAdminActor) error) error
	WithAdminSession(context.Context, identity.PlatformAdminActor, func(context.Context) error) error
}

// Management owns a dedicated Redis keyspace on the ordinary store's shared
// client, password gate and connection budget. It never takes an ordinary slot.
type Management struct {
	store  *Store
	roles  ManagementAuthority
	policy Policy
}

// NewManagement uses the fixed platform administration deadlines.
func NewManagement(store *Store, roles ManagementAuthority) (*Management, error) {
	return newManagement(store, roles, managementPolicy)
}

func newManagement(store *Store, roles ManagementAuthority, policy Policy) (*Management, error) {
	if store == nil || store.client == nil || store.gate == nil || store.authority == nil || roles == nil ||
		!policy.valid() || policy.Idle > managementPolicy.Idle || policy.Absolute > managementPolicy.Absolute {
		return nil, ErrInvalid
	}
	return &Management{store: store, roles: roles, policy: policy}, nil
}

func (s *Management) check(ctx context.Context) error {
	if s == nil || s.store == nil || s.roles == nil {
		return ErrInvalid
	}
	return s.store.check(ctx)
}

func (s *Management) tokenKey(c Credential) string {
	return "oj-management:" + s.store.namespace + ":token:" + digest(c.token)
}

// Login verifies the password independently of any ordinary session. The
// caller must charge the shared login limiter before calling. Issuance is one
// shot; a missing receipt never permits retry with the same credential.
func (s *Management) Login(ctx context.Context, email identity.Email, secret []byte) (Credential, error) {
	var zero Credential
	if err := s.check(ctx); err != nil {
		return zero, err
	}
	if !email.Valid() {
		return zero, ErrInvalid
	}
	personal, err := s.store.authority.LookupPersonalByEmail(ctx, email)
	if errors.Is(err, identity.ErrNotFound) {
		return zero, identity.ErrCredentials
	}
	if err != nil {
		return zero, err
	}
	role, err := s.roles.Inspect(ctx, personal.ID)
	if err != nil && !errors.Is(err, identity.ErrNotFound) {
		return zero, err
	}
	var checked identity.SessionState
	if err := s.store.gate.WithPersonalCredentials(ctx, email, secret, func(_ context.Context, state identity.SessionState) error {
		checked = state
		return nil
	}); err != nil {
		return zero, err
	}
	// Run the same password gate for known personal subjects whether or not
	// they hold the role. A pre-verification role lookup is only a snapshot.
	if !role.Enabled || role.SubjectID != checked.ID || role.Revision <= 0 {
		return zero, identity.ErrCredentials
	}
	var entropy [32]byte
	if _, err := io.ReadFull(s.store.random, entropy[:]); err != nil {
		return zero, ErrRandom
	}
	c := Credential{token: base64.RawURLEncoding.EncodeToString(entropy[:])}
	var receipt managementRecord
	err = s.roles.WithAdminAdmission(ctx, checked, role.Revision, func(call context.Context, actor identity.PlatformAdminActor) error {
		row, err := s.run(call, "issue", c, managementPrefix(actor, c))
		if err == nil && row.actor != actor {
			return storageError{ErrConflict}
		}
		receipt = row
		return err
	})
	if err != nil {
		return zero, err
	}
	err = s.roles.WithAdminAdmission(ctx, checked, role.Revision, func(call context.Context, actor identity.PlatformAdminActor) error {
		row, err := s.run(call, "read", c, receipt.raw)
		if err == nil && (row.raw != receipt.raw || row.actor != actor) {
			return storageError{ErrConflict}
		}
		return err
	})
	if err != nil {
		return zero, err
	}
	return c, nil
}

// ManagementView supplies an authenticated management actor and deadlines.
// Business operations must recheck current authority in their own transaction.
type ManagementView struct {
	Actor                           identity.PlatformAdminActor
	AbsoluteDeadline, IdleExpiresAt time.Time
}

// Read observes a management session without renewing it.
func (s *Management) Read(ctx context.Context, c Credential) (ManagementView, error) {
	return s.use(ctx, c, "read")
}

// Touch renews only the idle window, capped by the original absolute deadline.
func (s *Management) Touch(ctx context.Context, c Credential) (ManagementView, error) {
	return s.use(ctx, c, "touch")
}

// Revoke removes the original management credential and confirms deletion.
func (s *Management) Revoke(ctx context.Context, c Credential) error {
	_, err := s.use(ctx, c, "revoke")
	return err
}

func (s *Management) use(ctx context.Context, c Credential, op string) (ManagementView, error) {
	var view ManagementView
	if err := s.check(ctx); err != nil {
		return view, err
	}
	if _, err := ParseCredential(c.token); err != nil {
		return view, err
	}
	seen, err := s.run(ctx, "locate", c, "")
	if err != nil {
		return view, err
	}
	err = s.roles.WithAdminSession(ctx, seen.actor, func(call context.Context) error {
		row, err := s.run(call, op, c, seen.raw)
		if err != nil {
			return err
		}
		if op != "revoke" {
			if row.actor != seen.actor || row.created != seen.created || row.absolute != seen.absolute {
				return storageError{ErrConflict}
			}
			view = ManagementView{Actor: row.actor, AbsoluteDeadline: time.UnixMilli(row.absolute).UTC(), IdleExpiresAt: time.UnixMilli(row.idle).UTC()}
		}
		return nil
	})
	if err != nil {
		return ManagementView{}, err
	}
	return view, nil
}

type managementRecord struct {
	raw                     string
	actor                   identity.PlatformAdminActor
	created, absolute, idle int64
}

func managementPrefix(actor identity.PlatformAdminActor, c Credential) string {
	return strings.Join([]string{"1", digest(c.token), actor.SubjectID, strconv.FormatInt(actor.AuthGeneration, 10),
		actor.RecoveryID, strconv.FormatInt(actor.RoleRevision, 10), ""}, ":")
}

func (s *Management) decode(raw string, c Credential) (managementRecord, error) {
	var zero managementRecord
	fields := strings.Split(raw, ":")
	if len(raw) > managementMaxRecord || len(fields) != 9 || fields[0] != "1" || fields[1] != digest(c.token) {
		return zero, storageError{ErrConflict}
	}
	positive := func(raw string) (int64, bool) {
		n, err := strconv.ParseInt(raw, 10, 64)
		return n, err == nil && n > 0 && strconv.FormatInt(n, 10) == raw
	}
	auth, a := positive(fields[3])
	role, b := positive(fields[5])
	created, d := positive(fields[6])
	absolute, e := positive(fields[7])
	idle, f := positive(fields[8])
	actor := identity.PlatformAdminActor{SubjectID: fields[2], RoleRevision: role, SessionVersion: identity.SessionVersion{AuthGeneration: auth, RecoveryID: fields[4]}}
	if !a || !b || !d || !e || !f || !actor.Valid() || absolute > 9007199254740991 ||
		absolute-created != s.policy.Absolute.Milliseconds() || idle < created+s.policy.Idle.Milliseconds() || idle > absolute {
		return zero, storageError{ErrConflict}
	}
	return managementRecord{raw: raw, actor: actor, created: created, absolute: absolute, idle: idle}, nil
}

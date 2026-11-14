//go:build integration

package sessionprotocol_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type adminWriteBarrier struct {
	*identitypg.PlatformAdmins
	before func()
}

func (s adminWriteBarrier) ChangeAsAdmin(ctx context.Context, actor identity.PlatformAdminActor, version int64, req identity.PlatformAdminRoleRequest, confirm func(context.Context) error) (identity.PlatformAdminRole, error) {
	s.before()
	return s.PlatformAdmins.ChangeAsAdmin(ctx, actor, version, req, confirm)
}

func testManagementRoleWriteFences(t *testing.T, f *fixture) {
	for _, scenario := range []string{"logout", "password replacement", "revoke and regrant", "recovery rotation"} {
		t.Run(scenario, func(t *testing.T) {
			ns := namespace(t)
			a, b := f.subject(t, false), f.subject(t, false)
			_, err := f.pools[0].Exec(f.ctx, `UPDATE identity_personal_emails SET email_verified=true WHERE subject_id IN ($1,$2)`, a.ID, b.ID)
			must(t, err)
			roles, err := identitypg.NewPlatformAdmins(f.pools[0])
			must(t, err)
			mutation := identity.PlatformAdminChange{SubjectID: a.ID, Enabled: true, Operator: "fixture", Reason: "bootstrap"}
			_, err = roles.Change(f.ctx, mutation)
			must(t, err)
			store := f.store(t, ns, 0, protocol.OrdinaryPolicy())
			sessions, err := protocol.NewManagement(store, roles)
			must(t, err)
			peer, err := protocol.NewManagement(f.store(t, ns, 1, protocol.OrdinaryPolicy()), roles)
			must(t, err)
			secret := []byte("synthetic protocol password")
			credential, err := sessions.Login(f.ctx, a.Email, secret)
			must(t, err)
			limiter := limitedManager(t, f, store, loadedLoginSecrets(t, loginKey(t), loginKey(t)), rateLimits(), true)
			fired := false
			barrier := adminWriteBarrier{roles, func() {
				fired = true
				switch scenario {
				case "logout":
					must(t, peer.Revoke(f.ctx, credential))
				case "password replacement":
					record, err := f.hasher.Create([]byte("new test password"))
					must(t, err)
					_, err = f.identities[1].ReplacePasswordCAS(f.ctx, a.ID, a.Revision, record)
					must(t, err)
				case "revoke and regrant":
					mutation.ExpectedRevision, mutation.Enabled = 1, false
					_, err := roles.Change(f.ctx, mutation)
					must(t, err)
					mutation.ExpectedRevision, mutation.Enabled = 2, true
					_, err = roles.Change(f.ctx, mutation)
					must(t, err)
				case "recovery rotation":
					f.open(t)
				}
			}}
			service, err := protocol.NewManagementRoleChanges(sessions, limiter, barrier)
			must(t, err)
			result, err := service.Change(f.ctx, credential, netip.MustParseAddr("192.0.2.11"), secret,
				identity.PlatformAdminRoleRequest{SubjectID: b.ID, Enabled: true, Reason: "protected write"})
			want := error(identity.ErrPlatformAdminRequired)
			if scenario == "logout" {
				want = protocol.ErrStale
			}
			wantError(t, err, want)
			if !fired || result != (identity.PlatformAdminRole{}) {
				t.Fatalf("%s mutation was not enforced at final write", scenario)
			}
			_, err = roles.Inspect(f.ctx, b.ID)
			wantError(t, err, identity.ErrNotFound)
		})
	}
}

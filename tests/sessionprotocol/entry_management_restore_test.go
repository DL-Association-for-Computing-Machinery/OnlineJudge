//go:build integration

package sessionprotocol_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type managementRestore struct {
	ctx                 context.Context
	kept, revoked       identity.Personal
	roles               *identitypg.PlatformAdmins
	oldKept, oldRevoked *http.Cookie
	change              identity.PlatformAdminChange
}

func prepareManagementRestore(t *testing.T, f *fixture) *managementRestore {
	t.Helper()
	kept, revoked := f.subject(t, false), f.subject(t, false)
	_, err := f.pools[0].Exec(f.ctx, `UPDATE identity_personal_emails SET email_verified=true WHERE subject_id IN ($1,$2)`, kept.ID, revoked.ID)
	must(t, err)
	roles, err := identitypg.NewPlatformAdmins(f.pools[0])
	must(t, err)
	for _, p := range []identity.Personal{kept, revoked} {
		_, err := roles.Change(f.ctx, identity.PlatformAdminChange{SubjectID: p.ID, Enabled: true, Operator: "restore-fixture", Reason: "archived grant"})
		must(t, err)
	}
	return &managementRestore{ctx: f.ctx, kept: kept, revoked: revoked, roles: roles,
		change: identity.PlatformAdminChange{SubjectID: revoked.ID, ExpectedRevision: 1, Operator: "restore-fixture", Reason: "post-backup revocation"}}
}

func managementRestoreLogin(t *testing.T, e *loginEndpoint, p identity.Personal, status int) *http.Cookie {
	t.Helper()
	r := loginCall(e, managementPath+"/login", loginBodyFor(t, p), nil, nil)
	loginStatus(t, r, status)
	if status != 204 {
		return nil
	}
	if len(r.cookies) != 1 || r.cookies[0].Name != transport.ManagementCookieName {
		t.Fatal("management login did not issue an independent cookie")
	}
	return r.cookies[0]
}

func (f *managementRestore) signIn(t *testing.T, e *loginEndpoint) {
	t.Helper()
	f.oldKept = managementRestoreLogin(t, e, f.kept, 204)
	f.oldRevoked = managementRestoreLogin(t, e, f.revoked, 204)
}

func (f *managementRestore) revokeAtSource(t *testing.T) {
	t.Helper()
	_, err := f.roles.Change(f.ctx, f.change)
	must(t, err)
}

func (f *managementRestore) checkArchivedAuthority(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ordinary *protocol.Store) {
	t.Helper()
	roles, err := identitypg.NewPlatformAdmins(pool)
	must(t, err)
	management, err := protocol.NewManagement(ordinary, roles)
	must(t, err)
	for _, old := range []*http.Cookie{f.oldKept, f.oldRevoked} {
		credential, err := protocol.ParseCredential(old.Value)
		must(t, err)
		view, err := management.Read(ctx, credential)
		must(t, err)
		if view.Actor.RoleRevision != 1 {
			t.Fatal("archive did not restore the earlier management grant")
		}
	}
}

func (f *managementRestore) reconcile(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	roles, err := identitypg.NewPlatformAdmins(pool)
	must(t, err)
	after, err := roles.Change(ctx, f.change)
	must(t, err)
	if after.Enabled || after.Revision != 2 {
		t.Fatal("restore reconciliation did not reapply the post-backup role revocation")
	}
}

func (f *managementRestore) checkReopened(t *testing.T, x, y *loginEndpoint) {
	t.Helper()
	body := fmt.Sprintf(`{"subject_id":%q,"expected_revision":2,"enabled":true,"reason":"old cookie probe","password":"synthetic protocol password"}`, f.revoked.ID)
	for _, endpoint := range []*loginEndpoint{x, y} {
		for _, old := range []*http.Cookie{f.oldKept, f.oldRevoked} {
			assertManagementRead(t, endpoint, managementPath, old, 401)
			loginStatus(t, loginCall(endpoint, transport.ManagementRolePath, body, []*http.Cookie{old}, nil), 401)
		}
	}
	managementRestoreLogin(t, x, f.revoked, 401)
	current := managementRestoreLogin(t, x, f.kept, 204)
	if view := assertManagementRead(t, y, managementPath, current, 200); view.SubjectID != f.kept.ID {
		t.Fatal("fresh management session lost reconciled authority")
	}
}

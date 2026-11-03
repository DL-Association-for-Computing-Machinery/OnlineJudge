//go:build integration

package sessionprotocol_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

type draftMaterialHook struct {
	materialstore.Store
	once     sync.Once
	after    func()
	puts     int
	afterRan bool
}

func (s *draftMaterialHook) Put(ctx context.Context, o materialstore.Object, r io.Reader) (materialstore.Object, error) {
	got, err := s.Store.Put(ctx, o, r)
	if err == nil {
		s.puts++
		s.once.Do(func() { s.afterRan = true; s.after() })
	}
	return got, err
}

func testPrivateDraftSessions(t *testing.T, f *fixture) {
	for _, mode := range []string{"success", "default-deny", "logout-during-materials", "expiry-during-materials", "redis-client-failure", "role-revoked-during-materials"} {
		t.Run(mode, func(t *testing.T) {
			p := f.subject(t, false)
			policy := protocol.OrdinaryPolicy()
			if mode == "expiry-during-materials" {
				policy = protocol.Policy{Idle: 300 * time.Millisecond, Absolute: 2 * time.Second}
			}
			ns := namespace(t)
			sessions := f.store(t, ns, 0, policy)
			_, credential := f.create(t, sessions, p)
			access, err := identitypg.NewProblemAccess(f.pools[0])
			must(t, err)
			repository, err := problempg.New(f.pools[0], access)
			must(t, err)
			if mode != "default-deny" {
				_, err = access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: p.ID, ExpectedRevision: 0, Enabled: true, Operator: "protocol-fixture", Reason: "synthetic private draft"})
				must(t, err)
			}
			bundle := problemfixture.New(t)
			prepared, err := problem.LoadBundle(bundle.Directory, "library-checker:aplusb", bundle.Lock)
			must(t, err)
			defer func() { must(t, prepared.Close()) }()
			root := filepath.Join(t.TempDir(), "objects")
			must(t, os.Mkdir(root, 0700))
			store, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
			must(t, err)
			defer func() { must(t, store.Close()) }()
			materials := &draftMaterialHook{Store: store, after: func() {}}
			view, err := sessions.Read(f.ctx, credential)
			must(t, err)
			switch mode {
			case "logout-during-materials":
				materials.after = func() { must(t, sessions.Revoke(f.ctx, credential)) }
			case "expiry-during-materials":
				materials.after = func() { f.waitTime(t, view.IdleExpiresAt.Add(time.Millisecond)) }
			case "redis-client-failure":
				// A separate real client is closed only after the successful initial
				// Read and author preflight, leaving all other fixture clients intact.
				client, e := protocol.NewClient(protocol.ClientConfig{Addr: os.Getenv("OJ_PROTOCOL_REDIS_ADDR"), Connections: 2, Timeout: time.Second})
				must(t, e)
				sessions, e = protocol.New(f.ctx, client, f.identities[0], f.budget, ns, policy)
				must(t, e)
				defer func() { _ = client.Close() }()
				materials.after = func() { must(t, client.Close()) }
			case "role-revoked-during-materials":
				materials.after = func() {
					_, e := access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: p.ID, ExpectedRevision: 1, Enabled: false, Operator: "protocol-fixture", Reason: "revoke during material copy"})
					must(t, e)
				}
			}
			service, err := problem.New(sessions, repository, materials)
			must(t, err)
			draft, err := service.Create(f.ctx, credential, "protocol-private-draft-key", prepared)
			if mode == "default-deny" {
				if materials.puts != 0 || materials.afterRan {
					t.Fatal("ungranted account reached material I/O")
				}
			} else if materials.puts != len(prepared.Content().References) || !materials.afterRan {
				t.Fatal("final session boundary was not reached after real material persistence")
			}
			if mode == "success" {
				must(t, err)
				other, err := problempg.New(f.pools[1], access)
				must(t, err)
				second, err := problem.New(sessions, other, store)
				must(t, err)
				got, err := second.GetOwn(f.ctx, credential, draft.ID)
				must(t, err)
				if got.ID != draft.ID || len(got.References) != len(prepared.Content().References) {
					t.Fatal("real session owner metadata lost")
				}
			} else {
				want := error(protocol.ErrStale)
				if mode == "default-deny" || mode == "role-revoked-during-materials" {
					want = identity.ErrForbidden
				}
				if mode == "redis-client-failure" {
					want = protocol.ErrStore
				}
				if !errors.Is(err, want) {
					t.Fatalf("mode=%s expected=%v got=%v", mode, want, err)
				}
			}
			var count int
			must(t, f.pools[0].QueryRow(f.ctx, `SELECT count(*) FROM problem_drafts WHERE owner_subject_id=$1`, p.ID).Scan(&count))
			if (mode == "success" && count != 1) || (mode != "success" && count != 0) {
				t.Fatal("session failure created private metadata")
			}
		})
	}
	t.Log("real Redis credentials and PG/material store verified fresh session read, logout/expiry/store failure, default deny and role revocation")
}

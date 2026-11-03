//go:build integration

package sessionprotocol_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

type previewMaterialHook struct {
	materialstore.Store
	once  sync.Once
	after func()
	opens int
}

func (s *previewMaterialHook) Open(ctx context.Context, object materialstore.Object) (*materialstore.Snapshot, error) {
	snapshot, err := s.Store.Open(ctx, object)
	if err == nil {
		s.opens++
		s.once.Do(s.after)
	}
	return snapshot, err
}

func testPrivatePreviewSessions(t *testing.T, f *fixture) {
	for _, mode := range []string{"success", "logout-during-read", "expiry-during-read", "role-revoked-during-read", "account-disabled-during-read", "recovery-during-read"} {
		t.Run(mode, func(t *testing.T) {
			p := f.subject(t, false)
			sessions := f.store(t, namespace(t), 0, protocol.OrdinaryPolicy())
			_, credential := f.create(t, sessions, p)
			access, err := identitypg.NewProblemAccess(f.pools[0])
			must(t, err)
			_, err = access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: p.ID, ExpectedRevision: 0, Enabled: true, Operator: "preview-protocol", Reason: "synthetic preview author"})
			must(t, err)
			repository, err := problempg.New(f.pools[0], access)
			must(t, err)
			bundle := problemfixture.New(t)
			bundle.AddStatementImage(t, "figure.png", problemfixture.PNG(t, 80, 40))
			bundle.Write(t)
			prepared, err := problem.LoadBundle(bundle.Directory, "library-checker:aplusb", bundle.Lock)
			must(t, err)
			defer func() { must(t, prepared.Close()) }()
			root := filepath.Join(t.TempDir(), "objects")
			must(t, os.Mkdir(root, 0700))
			store, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
			must(t, err)
			defer func() { must(t, store.Close()) }()
			materials := &previewMaterialHook{Store: store, after: func() {}}
			service, err := problem.New(sessions, repository, materials)
			must(t, err)
			importer, err := problem.NewImporter(service, map[string]*problem.Prepared{"library-checker:aplusb": prepared}, 1)
			must(t, err)
			draft, err := importer.Import(f.ctx, credential, "preview-protocol-original-key", "library-checker:aplusb")
			must(t, err)
			if mode == "expiry-during-read" {
				// Begin the short test lifetime only AFTER import/fsync, using a
				// separate namespace so one namespace never mixes session policies.
				sessions = f.store(t, namespace(t)+"_short", 0, protocol.Policy{Idle: 300 * time.Millisecond, Absolute: 2 * time.Second})
				service, err = problem.New(sessions, repository, materials)
				must(t, err)
				importer, err = problem.NewImporter(service, map[string]*problem.Prepared{"library-checker:aplusb": prepared}, 1)
				must(t, err)
				_, credential = f.create(t, sessions, p)
			}
			view, err := sessions.Read(f.ctx, credential)
			must(t, err)
			want := error(nil)
			switch mode {
			case "logout-during-read":
				want = protocol.ErrStale
				materials.after = func() { must(t, sessions.Revoke(f.ctx, credential)) }
			case "expiry-during-read":
				want = protocol.ErrStale
				materials.after = func() { f.waitTime(t, view.IdleExpiresAt.Add(time.Millisecond)) }
			case "role-revoked-during-read":
				want = identity.ErrForbidden
				materials.after = func() {
					_, err := access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: p.ID, ExpectedRevision: 1, Enabled: false, Operator: "preview-protocol", Reason: "revoke during verified snapshot"})
					must(t, err)
				}
			case "account-disabled-during-read":
				want = identity.ErrConflict
				materials.after = func() { _, err := f.identities[1].DisableCAS(f.ctx, p.ID, p.Revision); must(t, err) }
			case "recovery-during-read":
				want = identity.ErrConflict
				materials.after = func() { f.open(t) }
			}
			preview, err := importer.Preview(f.ctx, credential, draft.ID)
			if !errors.Is(err, want) || materials.opens != 4 {
				t.Fatalf("preview %s: error=%v verified opens=%d", mode, err, materials.opens)
			}
			if mode == "success" {
				if preview.Markdown != "# Synthetic A + B\n" || len(preview.Examples) != 1 || preview.Examples[0].Output != "3\n" || len(preview.Images) != 1 || preview.Images[0].Source != "figure.png" || preview.Images[0].Width != 80 {
					t.Fatal("real stored preview changed immutable text")
				}
			} else if !reflect.DeepEqual(preview, problem.Preview{}) {
				t.Fatal("authority change returned partial private text")
			}
		})
	}
	t.Log("real Redis/PG preview final-read fences reject logout, expiry, role/account revocation and recovery after verified material I/O")
}

//go:build integration && linux

package integration_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func TestReferenceRequestFreezesPersistedObjectBytes(t *testing.T) {
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	b := f.bundle
	b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", []byte("synthetic reference\n"))
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
	must(t, err)
	t.Cleanup(func() { must(t, p.Close()) })
	legacy, err := p.PrepareReference(f.ctx, problem.SmallReferenceLimits())
	must(t, err)

	root := filepath.Join(t.TempDir(), "objects")
	must(t, os.Mkdir(root, 0700))
	config := materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 1}
	materials, err := materialstore.OpenLocal(config)
	must(t, err)
	t.Cleanup(func() { must(t, materials.Close()) })
	sessions := fixedDraftSession{view: redisstore.View{SubjectID: f.actor.SubjectID, AuthGeneration: f.actor.AuthGeneration, RecoveryID: f.actor.RecoveryID}}
	service, err := problem.New(sessions, f.repository, materials)
	must(t, err)
	credential, err := redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	must(t, err)
	draft, err := service.Create(f.ctx, credential, "freeze-draft-original-key", p)
	must(t, err)
	access, err := problempg.NewReferenceAccess(f.access)
	must(t, err)
	requests, err := referencepg.New(f.a, access)
	must(t, err)
	must(t, referencepg.Configure(f.ctx, f.a, problemreference.Limits{Global: 1, Owner: 1}))
	catalog, err := problemreference.NewCatalog(map[string]*problem.Prepared{draft.SourceID: p})
	must(t, err)
	const key = "freeze-request-original-key"
	first, err := requests.Enqueue(f.ctx, f.actor, key, draft.ID, catalog)
	must(t, err)

	must(t, materials.Close())
	must(t, p.Close())
	must(t, f.prepared.Close())
	must(t, os.RemoveAll(b.Directory))

	// Reopen the committed metadata and the private object root independently.
	requestReader, err := referencepg.New(f.b, access)
	must(t, err)
	draftReader, err := problempg.New(f.b, f.access)
	must(t, err)
	receipt, err := requestReader.Receipt(f.ctx, f.actor, key)
	must(t, err)
	persisted, err := draftReader.GetOwn(f.ctx, f.actor, draft.ID)
	must(t, err)
	reopened, err := materialstore.OpenLocal(config)
	must(t, err)
	t.Cleanup(func() { must(t, reopened.Close()) })
	snapshot, err := receipt.FreezeDraft(f.ctx, persisted, reopened)
	must(t, err)
	t.Cleanup(func() { must(t, snapshot.Close()) })
	must(t, reopened.Close())
	raw, err := snapshot.ReadSource(f.ctx)
	must(t, err)
	in, answer, err := snapshot.ReadCase(f.ctx, 0)
	must(t, err)
	if snapshot.Plan().Digest() != legacy.Digest() || string(raw) != "synthetic reference\n" || string(in) != "1 2\n" || string(answer) != "3\n" {
		t.Fatal("frozen plan or bytes changed across restart and bundle removal")
	}
	again, err := requestReader.Receipt(f.ctx, f.actor, key)
	must(t, err)
	if !reflect.DeepEqual(first, again) || persisted.ValidationState != "not-run" {
		t.Fatal("freezing changed the request or draft state")
	}
}

//go:build linux

package problem

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

const previewID = "33333333333333333333333333333333"

type previewSessions struct {
	mu         sync.Mutex
	view       redisstore.View
	err        error
	credential redisstore.Credential
}

func (s *previewSessions) Read(_ context.Context, c redisstore.Credential) (redisstore.View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c != s.credential {
		return redisstore.View{}, redisstore.ErrInvalid
	}
	return s.view, s.err
}

type previewRepository struct {
	mu               sync.Mutex
	draft            Draft
	first, final     error
	gets             int
	changeFinalDraft bool
	changeSourceID   bool
}

func (*previewRepository) CheckAuthor(context.Context, identity.ProblemActor) error { return nil }
func (r *previewRepository) Create(context.Context, identity.ProblemActor, string, *Prepared) (Draft, error) {
	return r.draft, nil
}
func (r *previewRepository) GetOwn(_ context.Context, a identity.ProblemActor, id string) (Draft, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gets++
	if id != r.draft.ID || a.SubjectID != r.draft.Owner {
		return Draft{}, ErrNotFound
	}
	if r.gets == 1 && r.first != nil {
		return Draft{}, r.first
	}
	if r.gets > 1 && r.final != nil {
		return Draft{}, r.final
	}
	d := r.draft
	if r.gets > 1 && r.changeFinalDraft {
		d.Version = "different final version"
	}
	if r.gets > 1 && r.changeSourceID {
		d.SourceID = "library-checker:different"
	}
	return d, nil
}

type previewMaterials struct {
	materialstore.Store
	mu        sync.Mutex
	opened    []materialstore.Object
	failAt    int
	onOpen    func()
	onPut     func()
	closeRead bool
}

func (m *previewMaterials) Put(ctx context.Context, o materialstore.Object, r io.Reader) (materialstore.Object, error) {
	if m.onPut != nil {
		m.onPut()
	}
	return m.Store.Put(ctx, o, r)
}
func (m *previewMaterials) Open(ctx context.Context, o materialstore.Object) (*materialstore.Snapshot, error) {
	m.mu.Lock()
	m.opened = append(m.opened, o)
	n := len(m.opened)
	m.mu.Unlock()
	if n == m.failAt {
		return nil, materialstore.ErrIntegrity
	}
	s, err := m.Store.Open(ctx, o)
	if err == nil {
		if m.onOpen != nil {
			m.onOpen()
		}
		if m.closeRead {
			if err := s.Close(); err != nil {
				return nil, err
			}
		}
	}
	return s, err
}

type previewFixture struct {
	importer   *Importer
	sessions   *previewSessions
	repository *previewRepository
	materials  *previewMaterials
	local      *materialstore.Local
	bundle     *problemfixture.Bundle
	root       string
}

func newPreviewFixture(t *testing.T, change func(*problemfixture.Bundle)) previewFixture {
	t.Helper()
	b := problemfixture.New(t)
	if change != nil {
		change(b)
		b.Write(t)
	}
	p, err := LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closePrepared(t, p) })
	root := filepath.Join(t.TempDir(), "objects")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	local, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Error("preview retained an active material snapshot")
		}
	})
	if err := p.persist(t.Context(), local); err != nil {
		t.Fatal(err)
	}
	credential, err := redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	sessions := &previewSessions{credential: credential, view: redisstore.View{SubjectID: "11111111111111111111111111111111", AuthGeneration: 1, RecoveryID: "22222222222222222222222222222222"}}
	c := p.Content()
	repo := &previewRepository{draft: Draft{ID: previewID, Owner: sessions.view.SubjectID, SourceID: c.SourceID, Version: c.Version, State: "private-draft", ValidationState: "not-run", Document: c.Document, References: c.References}}
	materials := &previewMaterials{Store: local}
	s, err := New(sessions, repo, materials)
	if err != nil {
		t.Fatal(err)
	}
	i, err := NewImporter(s, map[string]*Prepared{c.SourceID: p}, 1)
	if err != nil {
		t.Fatal(err)
	}
	return previewFixture{i, sessions, repo, materials, local, b, root}
}

func previewTextRef(b *problemfixture.Bundle, raw []byte) map[string]any {
	hash := problemfixture.Hash(raw)
	path := "objects/sha256/" + hash
	b.Files[path] = raw
	return map[string]any{"path": path, "sha256": hash, "size": len(raw)}
}

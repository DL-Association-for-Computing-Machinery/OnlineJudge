package problem

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestImporterDoesNotExposeCatalogBeforeAuthority(t *testing.T) {
	bundle := problemfixture.New(t)
	p, err := LoadBundle(bundle.Directory, "library-checker:aplusb", bundle.Lock)
	if err != nil {
		t.Fatal(err)
	}
	defer closePrepared(t, p)
	credential, err := redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	view := redisstore.View{SubjectID: "11111111111111111111111111111111", AuthGeneration: 1, RecoveryID: "22222222222222222222222222222222"}
	for _, mode := range []string{"invalid-session", "no-author-role", "success"} {
		for _, source := range []string{"library-checker:aplusb", "unknown"} {
			t.Run(mode+"/"+source, func(t *testing.T) {
				sessions := &sequencedSessions{credential: credential, observations: []sessionObservation{{view: view}, {view: view}}}
				materials := &checkingMaterials{}
				repo := &serviceRepository{materials: materials}
				if mode == "invalid-session" {
					sessions.observations[0].err = redisstore.ErrStale
				}
				if mode == "no-author-role" {
					repo.preflight = identity.ErrForbidden
				}
				s, err := New(sessions, repo, materials)
				if err != nil {
					t.Fatal(err)
				}
				catalog := map[string]*Prepared{"library-checker:aplusb": p}
				i, err := NewImporter(s, catalog, 1)
				if err != nil {
					t.Fatal(err)
				}
				delete(catalog, "library-checker:aplusb") // Constructor detached caller's map.
				_, err = i.Import(t.Context(), credential, "original-importer-request", source)
				want := error(nil)
				switch {
				case mode == "invalid-session":
					want = redisstore.ErrStale
				case mode == "no-author-role":
					want = identity.ErrForbidden
				case source == "unknown":
					want = ErrUnknownSource
				}
				if !errors.Is(err, want) {
					t.Fatalf("expected %v got %v", want, err)
				}
				if want != nil && (materials.puts != 0 || repo.creates != 0) {
					t.Fatal("rejected import reached material or metadata write")
				}
				if len(i.slots) != 0 {
					t.Fatal("completed import retained capacity")
				}
			})
		}
	}
}

type fixedSession struct{ view redisstore.View }

func (s fixedSession) Read(context.Context, redisstore.Credential) (redisstore.View, error) {
	return s.view, nil
}

type importRepository struct{}

func (importRepository) CheckAuthor(context.Context, identity.ProblemActor) error { return nil }
func (importRepository) Create(context.Context, identity.ProblemActor, string, *Prepared) (Draft, error) {
	return Draft{}, nil
}
func (importRepository) GetOwn(context.Context, identity.ProblemActor, string) (Draft, error) {
	return Draft{}, nil
}

func (importRepository) Receipt(context.Context, identity.ProblemActor, string) (ImportReceipt, error) {
	return ImportReceipt{}, ErrNotFound
}

type blockedMaterial struct {
	checkingMaterials
	once             sync.Once
	entered, release chan struct{}
}

func (m *blockedMaterial) Put(ctx context.Context, object materialstore.Object, reader io.Reader) (materialstore.Object, error) {
	m.once.Do(func() { close(m.entered); <-m.release })
	return m.checkingMaterials.Put(ctx, object, reader)
}

func TestImporterRejectsOverlapAndReleasesCapacity(t *testing.T) {
	b := problemfixture.New(t)
	p, err := LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	defer closePrepared(t, p)
	m := &blockedMaterial{entered: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	release := func() { once.Do(func() { close(m.release) }) }
	defer release()
	s, err := New(fixedSession{redisstore.View{SubjectID: "11111111111111111111111111111111", AuthGeneration: 1, RecoveryID: "22222222222222222222222222222222"}}, importRepository{}, m)
	if err != nil {
		t.Fatal(err)
	}
	i, err := NewImporter(s, map[string]*Prepared{"library-checker:aplusb": p}, 1)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		_, err := i.Import(t.Context(), redisstore.Credential{}, "first-import-request-key", "library-checker:aplusb")
		finished <- err
	}()
	<-m.entered
	_, err = i.Import(t.Context(), redisstore.Credential{}, "second-import-request-key", "library-checker:aplusb")
	if !errors.Is(err, ErrBusy) {
		t.Fatal("overlapping request was queued or admitted")
	}
	release()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if _, err = i.Import(t.Context(), redisstore.Credential{}, "second-import-request-key", "library-checker:aplusb"); err != nil {
		t.Fatal("completed request retained capacity")
	}
}

package problem

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

type sessionObservation struct {
	view redisstore.View
	err  error
}
type sequencedSessions struct {
	observations []sessionObservation
	calls        int
	credential   redisstore.Credential
}

func (s *sequencedSessions) Read(_ context.Context, c redisstore.Credential) (redisstore.View, error) {
	if c != s.credential {
		return redisstore.View{}, errors.New("wrong credential forwarded")
	}
	i := s.calls
	s.calls++
	if i >= len(s.observations) {
		return redisstore.View{}, errors.New("unexpected session read")
	}
	return s.observations[i].view, s.observations[i].err
}

type serviceRepository struct {
	preflight, errorFinal error
	checks, creates, gets int
	materials             *checkingMaterials
}

func (r *serviceRepository) CheckAuthor(context.Context, identity.ProblemActor) error {
	r.checks++
	if r.materials.puts != 0 {
		panic("preflight happened after material I/O")
	}
	return r.preflight
}
func (r *serviceRepository) Create(_ context.Context, _ identity.ProblemActor, key string, p *Prepared) (Draft, error) {
	r.creates++
	return Draft{RequestKey: key, SourceID: p.Content().SourceID, Version: p.Content().Version, ImportRevision: p.Content().RequestSHA256}, r.errorFinal
}
func (r *serviceRepository) GetOwn(context.Context, identity.ProblemActor, string) (Draft, error) {
	r.gets++
	return Draft{}, r.errorFinal
}

func (r *serviceRepository) Receipt(context.Context, identity.ProblemActor, string) (ImportReceipt, error) {
	return ImportReceipt{}, r.errorFinal
}

func (r *serviceRepository) ListOwn(context.Context, identity.ProblemActor, *ListPosition) ([]ImportReceipt, error) {
	return nil, r.errorFinal
}

func TestCreateFreshAuthorizationBoundaries(t *testing.T) {
	credential, err := redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	view := redisstore.View{SubjectID: "11111111111111111111111111111111", AuthGeneration: 1, RecoveryID: "22222222222222222222222222222222"}
	for _, name := range []string{"success", "initial-session-denied", "default-deny", "material-failure", "logout-before-final-read", "expiry-before-final-read", "redis-unavailable", "actor-changed", "role-revoked-after-materials"} {
		t.Run(name, func(t *testing.T) {
			b := problemfixture.New(t)
			p, err := LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
			if err != nil {
				t.Fatal(err)
			}
			defer closePrepared(t, p)
			sessions := &sequencedSessions{credential: credential, observations: []sessionObservation{{view: view}, {view: view}}}
			materials := &checkingMaterials{}
			repo := &serviceRepository{materials: materials}
			want := error(nil)
			wantCreates := 0
			wantPuts := 10
			wantReads := 2
			switch name {
			case "success":
				wantCreates = 1
			case "initial-session-denied":
				want = redisstore.ErrStale
				sessions.observations[0].err = want
				wantPuts = 0
				wantReads = 1
			case "default-deny":
				want = identity.ErrForbidden
				repo.preflight = want
				wantPuts = 0
				wantReads = 1
			case "material-failure":
				want = materialstore.ErrUnavailable
				materials.fail = 2
				wantPuts = 2
				wantReads = 1
			case "logout-before-final-read", "expiry-before-final-read":
				want = redisstore.ErrStale
				sessions.observations[1].err = want
			case "redis-unavailable":
				want = redisstore.ErrStore
				sessions.observations[1].err = want
			case "actor-changed":
				want = identity.ErrConflict
				sessions.observations[1].view.AuthGeneration++
			case "role-revoked-after-materials":
				want = identity.ErrForbidden
				repo.errorFinal = want
				wantCreates = 1
			}
			s, err := New(sessions, repo, materials)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Create(context.Background(), credential, "same-original-request-key", p)
			if !errors.Is(err, want) || repo.creates != wantCreates || materials.puts != wantPuts || sessions.calls != wantReads {
				t.Fatalf("boundary=%s error=%v creates=%d puts=%d reads=%d", name, err, repo.creates, materials.puts, sessions.calls)
			}
		})
	}
}

func TestGetOwnPassesFreshSessionToRepository(t *testing.T) {
	c, err := redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	v := redisstore.View{SubjectID: "11111111111111111111111111111111", AuthGeneration: 1, RecoveryID: "22222222222222222222222222222222"}
	sessions := &sequencedSessions{credential: c, observations: []sessionObservation{{view: v}}}
	repo := &serviceRepository{materials: &checkingMaterials{}, errorFinal: identity.ErrForbidden}
	s, err := New(sessions, repo, repo.materials)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetOwn(context.Background(), c, "33333333333333333333333333333333"); !errors.Is(err, identity.ErrForbidden) || repo.gets != 1 || sessions.calls != 1 {
		t.Fatal("owner read skipped current authority")
	}
}

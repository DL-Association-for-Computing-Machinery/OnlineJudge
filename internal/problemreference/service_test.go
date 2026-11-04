package problemreference

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const serviceKey = "same-original-reference-key"
const serviceDraft = "33333333333333333333333333333333"

// These stubs check the application boundary only, not Redis revocation or SQL
// ownership/role locking. Those require the separate real-store integration.
type serviceSessions struct {
	credential redisstore.Credential
	view       redisstore.View
	err        error
	reads      int
	afterRead  func()
}

func (s *serviceSessions) Read(_ context.Context, credential redisstore.Credential) (redisstore.View, error) {
	s.reads++
	if credential != s.credential {
		return redisstore.View{}, redisstore.ErrInvalid
	}
	view, err := s.view, s.err
	if s.afterRead != nil {
		s.afterRead()
	}
	return view, err
}

type serviceRepository struct {
	actors     []identity.ProblemActor
	operations []string
	keys       []string
	draft      string
	catalog    *Catalog
	result     Request
	err        error
	afterCall  func()
}

func (r *serviceRepository) record(actor identity.ProblemActor, operation, key string) (Request, error) {
	r.actors = append(r.actors, actor)
	r.operations = append(r.operations, operation)
	r.keys = append(r.keys, key)
	if r.afterCall != nil {
		r.afterCall()
	}
	result := r.result
	result.Owner, result.RequestKey = actor.SubjectID, key
	return result, r.err
}

func (r *serviceRepository) Enqueue(_ context.Context, actor identity.ProblemActor, key, draft string, catalog *Catalog) (Request, error) {
	r.draft, r.catalog = draft, catalog
	return r.record(actor, "enqueue", key)
}
func (r *serviceRepository) Receipt(_ context.Context, actor identity.ProblemActor, key string) (Request, error) {
	return r.record(actor, "receipt", key)
}
func (r *serviceRepository) Cancel(_ context.Context, actor identity.ProblemActor, key string) (Request, error) {
	return r.record(actor, "cancel", key)
}

func newServiceFixture(t *testing.T) (*Service, *serviceSessions, *serviceRepository, redisstore.Credential) {
	t.Helper()
	credential, err := redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	sessions := &serviceSessions{credential: credential, view: redisstore.View{
		SubjectID: "11111111111111111111111111111111", AuthGeneration: 3,
		RecoveryID: "22222222222222222222222222222222"}}
	repo := &serviceRepository{result: Request{ID: strings.Repeat("6", 32), DraftID: serviceDraft, State: Queued,
		Specification: historicalSpecification([]byte(`{"id":"retired-profile"}`))}}
	catalog, err := NewCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(sessions, repo, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return service, sessions, repo, credential
}

func invokeService(s *Service, operation string, ctx context.Context, credential redisstore.Credential) (Request, error) {
	switch operation {
	case "enqueue":
		return s.Enqueue(ctx, credential, serviceKey, serviceDraft)
	case "receipt":
		return s.Receipt(ctx, credential, serviceKey)
	default:
		return s.Cancel(ctx, credential, serviceKey)
	}
}

func TestServicePassesOnlyFreshSessionIdentityAndSupportsRetiredOriginalKeys(t *testing.T) {
	s, sessions, repo, credential := newServiceFixture(t)
	for i, operation := range []string{"enqueue", "receipt", "cancel"} {
		if i == 1 {
			sessions.view.SubjectID = strings.Repeat("4", 32)
			sessions.view.AuthGeneration++
			sessions.view.RecoveryID = strings.Repeat("5", 32)
		}
		want := identity.ProblemActor{SubjectID: sessions.view.SubjectID,
			SessionVersion: identity.SessionVersion{AuthGeneration: sessions.view.AuthGeneration, RecoveryID: sessions.view.RecoveryID}}
		got, err := invokeService(s, operation, context.Background(), credential)
		if err != nil || got.ID != repo.result.ID || got.Owner != want.SubjectID || repo.actors[i] != want || repo.keys[i] != serviceKey || sessions.reads != i+1 {
			t.Fatal("operation ignored fresh session observation or original key", operation, err)
		}
	}
	if repo.draft != serviceDraft || repo.catalog != s.catalog || len(repo.catalog.sources) != 0 {
		t.Fatal("service substituted caller configuration or rejected retired-key recovery")
	}
	if !reflect.DeepEqual(repo.operations, []string{"enqueue", "receipt", "cancel"}) {
		t.Fatal("wrong repository operations")
	}
}

func TestServiceRejectsMalformedInputsBeforeSessionOrStorage(t *testing.T) {
	s, sessions, repo, credential := newServiceFixture(t)
	catalog, _ := NewCatalog(nil)
	for _, build := range []func() (*Service, error){
		func() (*Service, error) { return New(nil, repo, catalog) },
		func() (*Service, error) { return New(sessions, nil, catalog) },
		func() (*Service, error) { return New(sessions, repo, nil) },
	} {
		if got, err := build(); got != nil || !errors.Is(err, ErrInvalid) {
			t.Fatal("missing dependency accepted")
		}
	}
	for _, key := range []string{"", "short", "sixteen valid chars", strings.Repeat("a", 129)} {
		for _, operation := range []string{"enqueue", "receipt", "cancel"} {
			var err error
			switch operation {
			case "enqueue":
				_, err = s.Enqueue(context.Background(), credential, key, serviceDraft)
			case "receipt":
				_, err = s.Receipt(context.Background(), credential, key)
			case "cancel":
				_, err = s.Cancel(context.Background(), credential, key)
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatal("malformed key accepted", operation)
			}
		}
	}
	for _, draft := range []string{"", "bad", strings.Repeat("g", 32)} {
		if _, err := s.Enqueue(context.Background(), credential, serviceKey, draft); !errors.Is(err, ErrInvalid) {
			t.Fatal("malformed draft ID accepted")
		}
	}
	for _, operation := range []string{"enqueue", "receipt", "cancel"} {
		if _, err := invokeService(s, operation, nil, credential); !errors.Is(err, ErrInvalid) {
			t.Fatal("nil context accepted")
		}
		var absent *Service
		if _, err := invokeService(absent, operation, context.Background(), credential); !errors.Is(err, ErrInvalid) {
			t.Fatal("nil service accepted")
		}
		if _, err := invokeService(&Service{}, operation, context.Background(), credential); !errors.Is(err, ErrInvalid) {
			t.Fatal("zero service accepted")
		}
	}
	if sessions.reads != 0 || len(repo.actors) != 0 {
		t.Fatal("invalid request crossed authentication/storage boundary")
	}
}

func TestServiceSessionAndRepositoryFailuresNeverReturnReceipts(t *testing.T) {
	for _, operation := range []string{"enqueue", "receipt", "cancel"} {
		for _, mode := range []string{"bad-credential", "revoked", "redis-failure", "bad-subject", "bad-generation", "bad-recovery", "role-revoked", "store-failure", "commit-unknown"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				s, sessions, repo, credential := newServiceFixture(t)
				want, calls := error(ErrInvalid), 0
				switch mode {
				case "bad-credential":
					credential, want = redisstore.Credential{}, redisstore.ErrInvalid
				case "revoked":
					want, sessions.err = redisstore.ErrStale, redisstore.ErrStale
				case "redis-failure":
					want, sessions.err = redisstore.ErrStore, redisstore.ErrStore
				case "bad-subject":
					sessions.view.SubjectID = "bad"
				case "bad-generation":
					sessions.view.AuthGeneration = 0
				case "bad-recovery":
					sessions.view.RecoveryID = "bad"
				case "role-revoked":
					want, repo.err, calls = identity.ErrForbidden, identity.ErrForbidden, 1
				case "store-failure":
					want, repo.err, calls = ErrStore, ErrStore, 1
				case "commit-unknown":
					want, repo.err, calls = ErrCommitUnknown, ErrCommitUnknown, 1
				}
				got, err := invokeService(s, operation, context.Background(), credential)
				if !errors.Is(err, want) || !reflect.DeepEqual(got, Request{}) || sessions.reads != 1 || len(repo.actors) != calls {
					t.Fatal("failure returned data or skipped fresh authority", err)
				}
			})
		}
	}
}

func TestServiceCancellationAtSessionAndAfterSuccessfulRepository(t *testing.T) {
	for _, operation := range []string{"enqueue", "receipt", "cancel"} {
		for _, phase := range []string{"before-read", "after-read", "after-repository", "repository-error"} {
			t.Run(operation+"/"+phase, func(t *testing.T) {
				s, sessions, repo, credential := newServiceFixture(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				reads, calls := 1, 0
				switch phase {
				case "before-read":
					cancel()
					reads = 0
				case "after-read":
					sessions.afterRead = cancel
				case "after-repository":
					repo.afterCall = cancel
					calls = 1
				case "repository-error":
					repo.afterCall, repo.err, calls = cancel, ErrConflict, 1
				}
				got, err := invokeService(s, operation, ctx, credential)
				unknown := phase == "after-repository" && operation != "receipt"
				if !errors.Is(err, context.Canceled) || errors.Is(err, ErrCommitUnknown) != unknown || !reflect.DeepEqual(got, Request{}) || sessions.reads != reads || len(repo.actors) != calls {
					t.Fatal("context boundary or commit-unknown contract changed", err)
				}
				if phase == "repository-error" && !errors.Is(err, ErrConflict) {
					t.Fatal("independent repository error was lost", err)
				}
			})
		}
	}
}

func TestServiceKeepsAcceptedInFlightLogoutBoundary(t *testing.T) {
	s, sessions, repo, credential := newServiceFixture(t)
	repo.afterCall = func() { sessions.err = redisstore.ErrStale }
	got, err := s.Enqueue(context.Background(), credential, serviceKey, serviceDraft)
	if err != nil || got.ID == "" || sessions.reads != 1 || len(repo.actors) != 1 {
		t.Fatal("service invented an atomic session/storage logout boundary", err)
	}
	if got, err := s.Receipt(context.Background(), credential, serviceKey); !errors.Is(err, redisstore.ErrStale) || !reflect.DeepEqual(got, Request{}) || sessions.reads != 2 || len(repo.actors) != 1 {
		t.Fatal("next call reused a revoked observation", err)
	}
}

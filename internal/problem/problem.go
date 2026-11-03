// Package problem owns private draft metadata and authorization at its use cases.
package problem

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

var (
	ErrInvalid        = errors.New("invalid private draft request")
	ErrNotFound       = errors.New("private draft not found")
	ErrConflict       = errors.New("private draft request key conflict")
	ErrStore          = errors.New("private draft storage unavailable")
	ErrCommitUnknown  = errors.New("private draft commit outcome unknown; retain original request key")
	ErrBusy           = errors.New("private draft preparation in use")
	ErrSubjectChanged = errors.New("private draft session subject changed")
)

// Reference is a private logical role and immutable object identity. Its name
// is not a path accepted by the object store or a download authorization.
type Reference struct {
	Name   string               `json:"name"`
	Object materialstore.Object `json:"object"`
}

func (s *Service) actorForSubject(ctx context.Context, credential redisstore.Credential, expected string) (identity.ProblemActor, error) {
	a, err := s.actor(ctx, credential)
	if err != nil {
		return a, err
	}
	if expected != "" {
		if err := ctx.Err(); err != nil {
			return identity.ProblemActor{}, err
		}
	}
	if expected != "" && a.SubjectID != expected {
		return identity.ProblemActor{}, ErrSubjectChanged
	}
	return a, nil
}

// Content is a detached snapshot of validated import metadata.
type Content struct {
	SourceID      string
	Version       string
	RequestSHA256 string
	Document      []byte
	References    []Reference
}

// Draft is owner-scoped private metadata. Document references remain private.
type Draft struct {
	ID              string      `json:"id"`
	Owner           string      `json:"owner"`
	RequestKey      string      `json:"request_key"`
	SourceID        string      `json:"source_id"`
	Version         string      `json:"version"`
	ImportRevision  string      `json:"-"`
	State           string      `json:"state"`
	ValidationState string      `json:"validation_state"`
	Document        []byte      `json:"document"`
	References      []Reference `json:"references"`
	CreatedAt       time.Time   `json:"created_at"`
}

// Repository checks current identity and author permission in the SAME short
// transaction as every business read/write. CheckAuthor performs preflight only.
type Repository interface {
	CheckAuthor(context.Context, identity.ProblemActor) error
	Create(context.Context, identity.ProblemActor, string, *Prepared) (Draft, error)
	GetOwn(context.Context, identity.ProblemActor, string) (Draft, error)
	Receipt(context.Context, identity.ProblemActor, string) (ImportReceipt, error)
}

// SessionReader accepts only opaque ordinary-session credentials.
type SessionReader interface {
	Read(context.Context, redisstore.Credential) (redisstore.View, error)
}

// Service binds fresh session observations, current permissions and storage.
// It does not publish, share, execute, or grant general material downloads.
type Service struct {
	sessions   SessionReader
	repository Repository
	materials  materialstore.Store
}

// New requires the real session reader, authorized repository and private store.
func New(sessions SessionReader, repository Repository, materials materialstore.Store) (*Service, error) {
	if sessions == nil || repository == nil || materials == nil {
		return nil, ErrInvalid
	}
	return &Service{sessions: sessions, repository: repository, materials: materials}, nil
}

func (s *Service) actor(ctx context.Context, credential redisstore.Credential) (identity.ProblemActor, error) {
	v, err := s.sessions.Read(ctx, credential)
	if err != nil {
		return identity.ProblemActor{}, err
	}
	a := identity.ProblemActor{SubjectID: v.SubjectID, SessionVersion: identity.SessionVersion{AuthGeneration: v.AuthGeneration, RecoveryID: v.RecoveryID}}
	if !a.Valid() {
		return identity.ProblemActor{}, ErrInvalid
	}
	return a, nil
}

// Create persists all selected materials outside database locks, then reads the
// actual session again and atomically checks current identity/role and inserts
// metadata. Failed calls may leave unreferenced immutable objects; never delete
// those automatically. Retry uncertain outcomes using the SAME key and content.
// Single-token logout after the final Read follows the ordinary session's
// accepted-in-flight contract; no Redis/PostgreSQL atomic logout is claimed.
func (s *Service) Create(ctx context.Context, credential redisstore.Credential, key string, p *Prepared) (Draft, error) {
	if s == nil || ctx == nil || !ValidKey(key) || p == nil {
		return Draft{}, ErrInvalid
	}
	return s.create(ctx, credential, key, "", func() (*Prepared, error) { return p, nil })
}

func (s *Service) create(ctx context.Context, credential redisstore.Credential, key, expectedSubject string, selectPrepared func() (*Prepared, error)) (Draft, error) {
	actor, err := s.actorForSubject(ctx, credential, expectedSubject)
	if err != nil {
		return Draft{}, err
	}
	if err = s.repository.CheckAuthor(ctx, actor); err != nil {
		return Draft{}, err
	}
	p, err := selectPrepared()
	if err != nil {
		return Draft{}, err
	}
	if err = p.acquire(); err != nil {
		return Draft{}, err
	}
	defer p.release()
	if err = p.persist(ctx, s.materials); err != nil {
		return Draft{}, err
	}
	latest, err := s.actor(ctx, credential)
	if err != nil {
		return Draft{}, err
	}
	if latest != actor {
		return Draft{}, identity.ErrConflict
	}
	return s.repository.Create(ctx, latest, key, p)
}

// GetOwn rechecks the current author role, including for an existing owner.
func (s *Service) GetOwn(ctx context.Context, credential redisstore.Credential, id string) (Draft, error) {
	if s == nil || ctx == nil || !identity.ValidID(id) {
		return Draft{}, ErrInvalid
	}
	a, err := s.actor(ctx, credential)
	if err != nil {
		return Draft{}, err
	}
	return s.repository.GetOwn(ctx, a, id)
}

// ValidKey is a bounded owner-scoped opaque retry key, never a credential.
func ValidKey(key string) bool {
	if len(key) < 16 || len(key) > 128 {
		return false
	}
	for _, c := range key {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

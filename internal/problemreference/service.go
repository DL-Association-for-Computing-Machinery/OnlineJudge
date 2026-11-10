package problemreference

import (
	"context"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// SessionReader resolves opaque ordinary-session credentials without renewal.
type SessionReader interface {
	Read(context.Context, redisstore.Credential) (redisstore.View, error)
}

// Service binds each operation to a fresh session observation and a repository
// that rechecks current identity and author permission in its own transaction.
// Logout after the Read follows accepted-in-flight semantics; Redis and SQL are
// not an atomic permission boundary. This service neither judges nor publishes.
type Service struct {
	sessions   SessionReader
	repository Repository
	catalog    *Catalog
}

// New requires trusted configuration, including an explicit empty retired
// catalog when only existing-key receipts and cancellation should remain usable.
func New(sessions SessionReader, repo Repository, catalog *Catalog) (*Service, error) {
	if sessions == nil || repo == nil || catalog == nil {
		return nil, ErrInvalid
	}
	return &Service{sessions: sessions, repository: repo, catalog: catalog}, nil
}

// Enqueue records an intent for an owned draft and its exact trusted catalog
// selection. An uncertain result must be recovered with the same original key.
func (s *Service) Enqueue(ctx context.Context, credential redisstore.Credential, key, draftID string) (Request, error) {
	if !problem.ValidKey(key) || !identity.ValidID(draftID) {
		return Request{}, ErrInvalid
	}
	return s.call(ctx, credential, true, func(actor identity.ProblemActor) (Request, error) {
		return s.repository.Enqueue(ctx, actor, key, draftID, s.catalog)
	})
}

// Receipt observes a current owner's key independently of current source
// configuration. Absence does not prove that an earlier write never committed.
func (s *Service) Receipt(ctx context.Context, credential redisstore.Credential, key string) (Request, error) {
	if !problem.ValidKey(key) {
		return Request{}, ErrInvalid
	}
	return s.call(ctx, credential, false, func(actor identity.ProblemActor) (Request, error) {
		return s.repository.Receipt(ctx, actor, key)
	})
}

// Cancel asks the repository to cancel a queued intent under current authority.
// It does not stop a Worker or revoke an accepted in-flight session operation.
func (s *Service) Cancel(ctx context.Context, credential redisstore.Credential, key string) (Request, error) {
	if !problem.ValidKey(key) {
		return Request{}, ErrInvalid
	}
	return s.call(ctx, credential, true, func(actor identity.ProblemActor) (Request, error) {
		return s.repository.Cancel(ctx, actor, key)
	})
}

func (s *Service) call(ctx context.Context, credential redisstore.Credential, write bool, operation func(identity.ProblemActor) (Request, error)) (Request, error) {
	if s == nil || ctx == nil || s.sessions == nil || s.repository == nil || s.catalog == nil {
		return Request{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Request{}, err
	}
	actor, err := observeActor(ctx, s.sessions, credential)
	if err != nil {
		return Request{}, err
	}
	result, err := operation(actor)
	if err != nil {
		return Request{}, errors.Join(err, ctx.Err())
	}
	if err := ctx.Err(); err != nil {
		if write {
			return Request{}, errors.Join(ErrCommitUnknown, err)
		}
		return Request{}, err
	}
	return result, nil
}

func observeActor(ctx context.Context, sessions SessionReader, credential redisstore.Credential) (identity.ProblemActor, error) {
	view, err := sessions.Read(ctx, credential)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return identity.ProblemActor{}, err
	}
	actor := identity.ProblemActor{SubjectID: view.SubjectID,
		SessionVersion: identity.SessionVersion{AuthGeneration: view.AuthGeneration, RecoveryID: view.RecoveryID}}
	if !actor.Valid() {
		return identity.ProblemActor{}, ErrInvalid
	}
	return actor, nil
}

package problemreference

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// PlanRepository reads or writes plans under current authority in its own
// short transactions. It must compare the original authority on every write.
type PlanRepository interface {
	Receipt(context.Context, identity.ProblemActor, string) (Request, error)
	PersistExpectedPlan(context.Context, identity.ProblemActor, PreparedPlan) (ExpectedPlan, error)
	ExpectedPlan(context.Context, identity.ProblemActor, string) (ExpectedPlan, error)
}

// PlanDraftReader reads a current author's private draft under repository authorization.
type PlanDraftReader interface {
	GetOwn(context.Context, identity.ProblemActor, string) (problem.Draft, error)
}

// PlanService prepares immutable plan metadata for an ordinary session. It has
// no execution or reporting capability.
type PlanService struct {
	sessions  SessionReader
	requests  PlanRepository
	drafts    PlanDraftReader
	materials problem.ReferenceObjectReader
}

// NewPlanService binds authorized repositories and private material storage.
func NewPlanService(sessions SessionReader, requests PlanRepository, drafts PlanDraftReader, materials problem.ReferenceObjectReader) (*PlanService, error) {
	if sessions == nil || requests == nil || drafts == nil || materials == nil {
		return nil, ErrInvalid
	}
	return &PlanService{sessions, requests, drafts, materials}, nil
}

// Prepare reads the intent and draft, freezes materials outside SQL locks, and
// observes the same session again before the repository rechecks live authority
// and queue state. An unknown write outcome is recoverable by original key.
func (s *PlanService) Prepare(ctx context.Context, credential redisstore.Credential, key string) (ExpectedPlan, error) {
	actor, err := s.actor(ctx, credential, key)
	if err != nil {
		return ExpectedPlan{}, err
	}
	r, err := s.requests.Receipt(ctx, actor, key)
	if err != nil {
		return ExpectedPlan{}, err
	}
	if r.State != Queued || r.CancelledAt != nil {
		return ExpectedPlan{}, ErrNotQueued
	}
	if r.Owner != actor.SubjectID || r.RequestKey != key || r.Authorization.Actor != actor {
		return ExpectedPlan{}, ErrAuthorityChanged
	}
	draft, err := s.drafts.GetOwn(ctx, actor, r.DraftID)
	if err != nil {
		return ExpectedPlan{}, err
	}
	plan, err := s.freezePlan(ctx, r, draft)
	if err != nil {
		return ExpectedPlan{}, err
	}
	latest, err := s.actor(ctx, credential, key)
	if err != nil {
		return ExpectedPlan{}, err
	}
	if latest != actor {
		return ExpectedPlan{}, ErrAuthorityChanged
	}
	result, err := s.requests.PersistExpectedPlan(ctx, latest, PreparedPlan{r, plan})
	if err != nil {
		return ExpectedPlan{}, err
	}
	if err := ctx.Err(); err != nil {
		return ExpectedPlan{}, errors.Join(ErrCommitUnknown, err)
	}
	return result, nil
}

func (s *PlanService) freezePlan(ctx context.Context, r Request, draft problem.Draft) (ExpectedPlan, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	snapshot, err := r.FreezeDraft(ctx, draft, s.materials)
	if err != nil {
		return ExpectedPlan{}, err
	}
	plan := snapshot.Plan()
	if err := errors.Join(snapshot.Close(), ctx.Err()); err != nil {
		return ExpectedPlan{}, err
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return ExpectedPlan{}, ErrInvalid
	}
	return ExpectedPlan{Encoding: raw, SHA256: plan.Digest()}, nil
}

// Receipt is a fresh, read-only observation of an already recorded plan, even
// after cancellation. Its existence does not grant permission to execute.
func (s *PlanService) Receipt(ctx context.Context, credential redisstore.Credential, key string) (ExpectedPlan, error) {
	actor, err := s.actor(ctx, credential, key)
	if err != nil {
		return ExpectedPlan{}, err
	}
	return s.requests.ExpectedPlan(ctx, actor, key)
}

func (s *PlanService) actor(ctx context.Context, credential redisstore.Credential, key string) (identity.ProblemActor, error) {
	if s == nil || ctx == nil || s.sessions == nil || !problem.ValidKey(key) {
		return identity.ProblemActor{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return identity.ProblemActor{}, err
	}
	return observeActor(ctx, s.sessions, credential)
}

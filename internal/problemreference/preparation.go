package problemreference

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ErrPreparationBusy denotes a full process-local preparation slot.
var ErrPreparationBusy = errors.New("reference material preparation busy")

// Preparation runs at most one authorized job per process and has no waiting
// queue. The session credential stays only in the active job's closure.
type Preparation struct {
	plans  *PlanService
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	active string
	closed bool
	wg     sync.WaitGroup
}

// NewPreparation creates an inactive owner that must be closed before the
// caller closes its PlanService dependencies.
func NewPreparation(plans *PlanService) (*Preparation, error) {
	if plans == nil {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Preparation{plans: plans, ctx: ctx, cancel: cancel}, nil
}

// Start returns ready=true for an already durable plan. Otherwise it accepts
// at most one bounded job. A lost response does not cause an automatic retry.
func (p *Preparation) Start(ctx context.Context, credential redisstore.Credential, expected, key string) (ready bool, err error) {
	if p == nil || !identity.ValidID(expected) {
		return false, ErrInvalid
	}
	actor, request, err := p.plans.queuedIntent(ctx, credential, expected, key)
	if err != nil {
		return false, err
	}
	if ready, err := p.plans.planReady(ctx, actor, key); ready || err != nil {
		return ready, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if p.closed {
		return false, ErrPreparationBusy
	}
	if p.active == request.ID {
		return false, nil
	}
	if p.active != "" {
		return false, ErrPreparationBusy
	}
	p.active = request.ID
	p.wg.Go(func() {
		defer func() {
			p.mu.Lock()
			p.active = ""
			p.mu.Unlock()
		}()
		job, cancel := context.WithTimeout(p.ctx, 3*time.Minute)
		defer cancel()
		// A failure or unknown write leaves only the durable Plan authoritative.
		// The author may check it and explicitly submit the same key again.
		_, _ = p.plans.PrepareForSubject(job, credential, expected, key)
	})
	return false, nil
}

// Ready observes the durable plan; it does not report volatile work as ready.
func (p *Preparation) Ready(ctx context.Context, credential redisstore.Credential, expected, key string) (bool, error) {
	return p.plans.PlanReadyForSubject(ctx, credential, expected, key)
}

// Close cancels work and drains it before its repositories can be closed.
func (p *Preparation) Close() {
	p.mu.Lock()
	p.closed = true
	p.cancel()
	p.mu.Unlock()
	p.wg.Wait()
}

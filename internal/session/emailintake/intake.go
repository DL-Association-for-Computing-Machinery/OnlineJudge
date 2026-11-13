// Package emailintake bounds pre-issuance work for one API process. A successful
// handoff is volatile; the processor owns durable issuance and mail scheduling.
package emailintake

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
)

var (
	// ErrConfig indicates invalid limits or missing dependencies.
	ErrConfig = errors.New("invalid email intake configuration")
	// ErrUnavailable indicates the process cannot accept the request.
	ErrUnavailable = errors.New("email intake unavailable")
)

// Config sets the total in-flight capacity, worker count and maximum age from
// intake, including queue wait. All limits must be set explicitly.
type Config struct {
	Capacity, Workers int
	MaxAge            time.Duration
}

// Intake owns a fixed number of workers. Call Drain before closing dependencies.
// Do not copy it. Submit and Drain may run concurrently.
type Intake struct {
	recovery func(context.Context) (identity.RecoveryState, error)
	admit    func(context.Context, netip.Addr, identity.Email) error
	process  func(context.Context, identity.Email, emailtoken.Purpose, string)
	maxAge   time.Duration
	slots    chan struct{}
	jobs     chan job
	mu       sync.Mutex
	closing  bool
	submits  sync.WaitGroup
	workers  sync.WaitGroup
	drain    sync.Once
}

type job struct {
	source   netip.Addr
	email    identity.Email
	purpose  emailtoken.Purpose
	epoch    string
	deadline time.Time
}

// New starts the fixed worker set. Admission runs before any processing, and
// a non-nil admission error always stops that job. The processor receives the
// original recovery generation and must recheck it before account effects.
// Callbacks must honor their context and report outcomes without sensitive data.
// Recovery can run concurrently up to Capacity; admission and processing share
// at most Workers concurrent calls.
func New(c Config, recovery func(context.Context) (identity.RecoveryState, error),
	admit func(context.Context, netip.Addr, identity.Email) error,
	process func(context.Context, identity.Email, emailtoken.Purpose, string),
) (*Intake, error) {
	if c.Workers < 1 || c.Workers > 64 || c.Capacity < c.Workers || c.Capacity > 1024 ||
		c.MaxAge <= 0 || c.MaxAge > time.Minute || recovery == nil || admit == nil || process == nil {
		return nil, ErrConfig
	}
	i := &Intake{
		recovery: recovery, admit: admit, process: process, maxAge: c.MaxAge,
		slots: make(chan struct{}, c.Capacity), jobs: make(chan job, c.Capacity),
	}
	for range c.Workers {
		i.workers.Go(i.run)
	}
	return i, nil
}

// Submit matches the public HTTP intake callback. Success means only handoff,
// independent of mailbox-specific quotas, account state and delivery. After
// handoff, client cancellation cannot withdraw the job.
func (i *Intake) Submit(ctx context.Context, source netip.Addr, purpose emailtoken.Purpose, email identity.Email) error {
	source = source.Unmap()
	if i == nil || i.jobs == nil || ctx == nil || ctx.Err() != nil || !source.IsValid() || source.Zone() != "" ||
		source.IsUnspecified() || source.IsMulticast() || purpose.Lifetime() == 0 || !email.Valid() {
		return ErrUnavailable
	}
	deadline := time.Now().Add(i.maxAge)
	if !i.reserve() {
		return ErrUnavailable
	}
	defer i.submits.Done()
	accepted := false
	defer func() {
		if !accepted {
			<-i.slots
		}
	}()
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	state, err := i.recovery(ctx)
	if err != nil || ctx.Err() != nil || !state.Enabled || !identity.ValidRecoveryID(state.Generation) {
		return ErrUnavailable
	}
	// One slot belongs to this submitter, so the buffered send cannot block.
	i.jobs <- job{source: source, email: email, purpose: purpose, epoch: state.Generation, deadline: deadline}
	accepted = true
	return nil
}

func (i *Intake) reserve() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closing {
		return false
	}
	select {
	case i.slots <- struct{}{}:
		i.submits.Add(1)
		return true
	default:
		return false
	}
}

func (i *Intake) run() {
	for work := range i.jobs {
		i.execute(work)
	}
}

func (i *Intake) execute(work job) {
	defer func() { <-i.slots }()
	ctx, cancel := context.WithDeadline(context.Background(), work.deadline)
	defer cancel()
	if ctx.Err() == nil && i.admit(ctx, work.source, work.email) == nil && ctx.Err() == nil {
		i.process(ctx, work.email, work.purpose, work.epoch)
	}
}

// Drain stops new reservations and waits for every existing submission and
// worker. The caller can bound its wait around Drain but must keep dependencies
// open until it actually returns. Do not call Drain from a callback.
func (i *Intake) Drain() {
	if i == nil || i.jobs == nil {
		return
	}
	i.drain.Do(func() {
		i.mu.Lock()
		i.closing = true
		i.mu.Unlock()
		i.submits.Wait()
		close(i.jobs)
		i.workers.Wait()
	})
}

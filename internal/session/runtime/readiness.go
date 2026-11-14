package runtime

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
)

const readinessBudget = time.Second
const readinessCache = time.Second

// readinessProbe coalesces simultaneous observations without adding a
// background poller or extending the lifetime of a disconnected request.
type readinessProbe struct {
	check func(context.Context) bool
	mu    sync.Mutex
	last  bool
	until time.Time
	next  time.Time
	done  chan struct{}
}

// AllowsReadiness checks the configured HTTPS host and optional pinned proxy.
func (r *Runtime) AllowsReadiness(request *http.Request) bool {
	return r != nil && request != nil && httptransport.AllowsReadiness(r.handler, request)
}

func (p *readinessProbe) ready(ctx context.Context) bool {
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(parent, readinessBudget)
	defer cancel()
	p.mu.Lock()
	if time.Now().Before(p.until) {
		last := p.last
		p.mu.Unlock()
		return last && ctx.Err() == nil
	}
	if done := p.done; done != nil {
		p.mu.Unlock()
		select {
		case <-done:
			p.mu.Lock()
			last := p.last && time.Now().Before(p.until)
			p.mu.Unlock()
			return last && ctx.Err() == nil
		case <-ctx.Done():
			return false
		}
	}
	if time.Now().Before(p.next) {
		p.mu.Unlock()
		return false
	}
	p.next = time.Now().Add(readinessCache)
	p.done = make(chan struct{})
	p.mu.Unlock()
	last := false
	defer func() {
		p.mu.Lock()
		p.last = last
		p.until = time.Time{}
		// A disconnect is not a dependency observation, but still consumes
		// this interval's probe budget.
		if parent.Err() == nil {
			p.until = time.Now().Add(readinessCache)
		}
		close(p.done)
		p.done = nil
		p.mu.Unlock()
	}()
	last = p.check(ctx) && ctx.Err() == nil
	return last
}

// Ready observes the ordinary session dependencies without changing them.
// It shares the runtime drain and connection limits with ordinary requests.
func (r *Runtime) Ready(ctx context.Context) bool {
	if r == nil || r.probe == nil || !r.enter() {
		return false
	}
	defer r.leave()
	ready := r.probe.ready(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	return ready && !r.closing
}

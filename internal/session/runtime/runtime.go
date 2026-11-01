package runtime

import (
	"context"
	"net/http"
	"sync"
)

// Runtime exposes only the tracked HTTPS handler. Clients and the bare handler
// stay private so callers cannot bypass draining or close them during KDF work.
// Do not copy a Runtime. ServeHTTP and Close may run concurrently.
type Runtime struct {
	handler       http.Handler
	cleanup       func() error
	mu            sync.Mutex
	active        int
	closing       bool
	drained, done chan struct{}
	closeErr      error
}

func (*Runtime) String() string     { return "[redacted session runtime]" }
func (r *Runtime) GoString() string { return r.String() }

func newRuntime(handler http.Handler, cleanup func() error) *Runtime {
	return &Runtime{handler: handler, cleanup: cleanup, drained: make(chan struct{}), done: make(chan struct{})}
}

// ServeHTTP tracks the entire synchronous call, including work past HTTP timeout.
// Once closing begins every new request fails closed before transport/storage.
func (r *Runtime) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if r == nil || r.handler == nil {
		unavailable(w)
		return
	}
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		unavailable(w)
		return
	}
	r.active++
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.active--
		if r.closing && r.active == 0 {
			close(r.drained)
		}
		r.mu.Unlock()
	}()
	r.handler.ServeHTTP(w, request)
}

// Close irreversibly stops entry and starts one background drain/cleanup. Each
// caller waits with its own context. Timeout does not close active resources or
// reopen entry; later Close calls can await completion. Stop the listener first.
func (r *Runtime) Close(ctx context.Context) error {
	if r == nil || r.handler == nil || ctx == nil {
		return ErrConfig
	}
	r.mu.Lock()
	if !r.closing {
		r.closing = true
		if r.active == 0 {
			close(r.drained)
		}
		go func() {
			<-r.drained
			r.closeErr = r.cleanup()
			close(r.done)
		}()
	}
	r.mu.Unlock()
	select {
	case <-r.done:
		return r.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func unavailable(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"type":"about:blank","title":"会话服务暂时不可用","status":503,"code":"SESSION_UNAVAILABLE"}`))
}

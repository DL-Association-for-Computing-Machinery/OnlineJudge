package sandbox_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type relayDirection uint8

const (
	relayUp relayDirection = 1 << iota
	relayDown
)

// faultRelay forwards opaque bytes, including the original end-to-end mTLS.
// It holds at most one 16 KiB chunk per direction while blocked. Resume closes
// old streams before reopening admission, rather than replaying delayed TLS.
// This helper and its loopback tests need no privileged sandbox or credentials.
type faultRelay struct {
	listener net.Listener
	upstream string
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	close    sync.Once

	forwardMu sync.Mutex
	blocked   relayDirection
	release   chan struct{}
	upHeld    atomic.Int64
	downHeld  atomic.Int64

	lifeMu sync.Mutex
	conns  map[net.Conn]struct{}
}

func newFaultRelay(ctx context.Context, upstream string) (*faultRelay, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	r := &faultRelay{listener: l, upstream: upstream, ctx: ctx, cancel: cancel, conns: make(map[net.Conn]struct{})}
	r.wg.Add(1) // Accept remains counted until no more connection workers can start.
	go r.accept()
	return r, nil
}

func (r *faultRelay) endpoint() string { return "https://" + r.listener.Addr().String() }

// Block returns only after earlier writes finish. No writer can pass the mode
// check, unlock, and subsequently send a response across this fault barrier.
func (r *faultRelay) Block(directions relayDirection) {
	r.forwardMu.Lock()
	defer r.forwardMu.Unlock()
	if r.release != nil {
		close(r.release)
	}
	r.release = make(chan struct{})
	r.blocked = directions
}

func (r *faultRelay) Resume() {
	r.forwardMu.Lock()
	defer r.forwardMu.Unlock()
	r.closeStreams()
	r.blocked = 0
	if r.release != nil {
		close(r.release)
		r.release = nil
	}
}

func (r *faultRelay) Close() bool {
	r.close.Do(func() {
		r.cancel()
		_ = r.listener.Close() // Cancellation/closing all streams is followed by a join.
		r.closeStreams()
	})
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(5 * time.Second):
		return false
	}
}

func (r *faultRelay) closeStreams() {
	r.lifeMu.Lock()
	defer r.lifeMu.Unlock()
	for c := range r.conns {
		_ = c.Close() // Private sockets; copy workers observe closure and are joined.
	}
}

func (r *faultRelay) track(c net.Conn) bool {
	r.lifeMu.Lock()
	defer r.lifeMu.Unlock()
	if r.ctx.Err() != nil || len(r.conns) >= 32 {
		_ = c.Close() // Refuse excess or post-close admission without spawning work.
		return false
	}
	r.conns[c] = struct{}{}
	return true
}

func (r *faultRelay) forget(c net.Conn) {
	_ = c.Close() // Release only this relay-owned connection.
	r.lifeMu.Lock()
	delete(r.conns, c)
	r.lifeMu.Unlock()
}

func (r *faultRelay) accept() {
	defer r.wg.Done()
	for {
		c, err := r.listener.Accept()
		if err != nil {
			return
		}
		if !r.track(c) {
			continue
		}
		r.wg.Add(1)
		go r.connect(c)
	}
}

func (r *faultRelay) connect(client net.Conn) {
	defer r.wg.Done()
	defer r.forget(client)
	dialer := net.Dialer{Timeout: 2 * time.Second}
	server, err := dialer.DialContext(r.ctx, "tcp", r.upstream)
	if err != nil || !r.track(server) {
		return
	}
	defer r.forget(server)
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()
	done := make(chan struct{}, 2)
	go func() { r.copy(ctx, server, client, relayUp); done <- struct{}{} }()
	go func() { r.copy(ctx, client, server, relayDown); done <- struct{}{} }()
	<-done
	cancel()
	_ = client.Close() // Release the other reader before joining both directions.
	_ = server.Close()
	<-done
}

func (r *faultRelay) copy(ctx context.Context, dst, src net.Conn, direction relayDirection) {
	buf := make([]byte, 16*1024)
	for {
		n, readErr := src.Read(buf)
		if n > 0 && !r.forward(ctx, dst, buf[:n], direction) {
			return
		}
		if readErr != nil {
			return
		}
	}
}

func (r *faultRelay) forward(ctx context.Context, dst net.Conn, data []byte, direction relayDirection) bool {
	counted := false
	for {
		r.forwardMu.Lock()
		if ctx.Err() != nil {
			r.forwardMu.Unlock()
			return false
		}
		if r.blocked&direction == 0 {
			// A bounded write shares the mode lock with Block/Resume. Lifecycle
			// cancellation closes sockets without needing this lock.
			err := dst.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if err == nil {
				_, err = io.Copy(dst, bytes.NewReader(data))
			}
			r.forwardMu.Unlock()
			return err == nil
		}
		if !counted {
			if direction == relayUp {
				r.upHeld.Add(int64(len(data)))
			} else {
				r.downHeld.Add(int64(len(data)))
			}
			counted = true
		}
		release := r.release
		r.forwardMu.Unlock()
		select {
		case <-ctx.Done():
			return false
		case <-release:
		}
	}
}

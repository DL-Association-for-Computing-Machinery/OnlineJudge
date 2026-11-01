package apiapp

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type stopServer struct {
	shutdown func(context.Context) error
	closed   atomic.Int64
}

func (s *stopServer) Shutdown(ctx context.Context) error { return s.shutdown(ctx) }
func (s *stopServer) Close() error                       { s.closed.Add(1); return nil }

type closerFunc func(context.Context) error

func (f closerFunc) Close(ctx context.Context) error { return f(ctx) }

func TestShutdownUsesOneBudgetAndAlwaysStartsRuntimeClose(t *testing.T) {
	contexts := make(chan context.Context, 2)
	s := &stopServer{shutdown: func(ctx context.Context) error { contexts <- ctx; <-ctx.Done(); return ctx.Err() }}
	closer := closerFunc(func(ctx context.Context) error { contexts <- ctx; <-ctx.Done(); return ctx.Err() })
	var logs bytes.Buffer
	err := stop(s, closer, make(chan error), 20*time.Millisecond, slog.New(slog.NewJSONHandler(&logs, nil)))
	if !errors.Is(err, ErrShutdown) || s.closed.Load() != 1 || strings.Contains(logs.String(), "gracefully") {
		t.Fatal("incomplete shutdown reported success or left connections open")
	}
	a, b := <-contexts, <-contexts
	if a != b {
		t.Fatal("server and runtime received separate shutdown budgets")
	}
	if _, ok := a.Deadline(); !ok {
		t.Fatal("shutdown was unbounded")
	}
}

func TestGracefulMessageRequiresBothDrains(t *testing.T) {
	for _, runtimeErr := range []error{nil, errSessionCloseFailure} {
		s := &stopServer{shutdown: func(context.Context) error { return nil }}
		served := make(chan error, 1)
		served <- http.ErrServerClosed
		var logs bytes.Buffer
		err := stop(s, closerFunc(func(context.Context) error { return runtimeErr }), served, time.Second, slog.New(slog.NewJSONHandler(&logs, nil)))
		if runtimeErr == nil {
			if err != nil || !strings.Contains(logs.String(), "API stopped gracefully") {
				t.Fatal("confirmed drain not reported")
			}
		} else if err != ErrShutdown || strings.Contains(logs.String(), "gracefully") || s.closed.Load() != 1 {
			t.Fatal("runtime failure masked by network shutdown")
		}
	}
}

var errSessionCloseFailure = errors.New("synthetic resource close failure")

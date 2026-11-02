package localdev

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type closeFunc func(context.Context) error

func (f closeFunc) Close(ctx context.Context) error { return f(ctx) }

func TestShutdownStartsRuntimeDrainBeforeWaitingForHTTP(t *testing.T) {
	entered, closing, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-closing
		close(finished)
		w.WriteHeader(204)
	}))
	defer server.Close()
	clientResult := make(chan error, 1)
	go func() {
		res, err := server.Client().Get(server.URL)
		if err == nil {
			err = res.Body.Close()
		}
		clientResult <- err
	}()
	<-entered
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	r := closeFunc(func(ctx context.Context) error {
		close(closing)
		select {
		case <-finished:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err := stopServer(ctx, server.Config, r); err != nil {
		t.Fatal("shutdown waited for HTTP before starting runtime drain")
	}
	if err := <-clientResult; err != nil {
		t.Fatal("request did not drain normally")
	}
}

func TestShutdownDoesNotHideRuntimeCloseFailure(t *testing.T) {
	failure := errors.New("synthetic cleanup failure")
	if err := stopServer(t.Context(), &http.Server{}, closeFunc(func(context.Context) error { return failure })); !errors.Is(err, failure) {
		t.Fatal("cleanup error hidden")
	}
}

func TestShutdownDeadlineIsNotSuccessfulDrain(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := stopServer(ctx, &http.Server{}, closeFunc(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }))
	if !errors.Is(err, context.Canceled) {
		t.Fatal("canceled drain reported success")
	}
}

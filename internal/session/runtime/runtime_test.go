package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloseWaitsForActualHandlerReturn(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(release) }) }
	defer finish()
	var cleanups atomic.Int64
	r := newRuntime(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		close(entered)
		<-release // Models synchronous work which survives request cancellation.
		w.WriteHeader(204)
	}), func() error { cleanups.Add(1); return nil })
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	response := httptest.NewRecorder()
	returned := make(chan struct{})
	go func() {
		r.ServeHTTP(response, httptest.NewRequest("GET", "https://oj.invalid", nil).WithContext(requestCtx))
		close(returned)
	}()
	<-entered
	cancelRequest()
	deadline, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := r.Close(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("close did not report active drain timeout")
	}
	if cleanups.Load() != 0 {
		t.Fatal("resources closed while handler still running")
	}
	denied := httptest.NewRecorder()
	r.ServeHTTP(denied, httptest.NewRequest("GET", "https://oj.invalid", nil))
	if denied.Code != 503 || denied.Header().Get("Cache-Control") != "no-store" || len(denied.Header().Values("Set-Cookie")) != 0 {
		t.Fatal("closing runtime accepted new work or changed cookies")
	}
	results := make(chan error, 16)
	for range 16 {
		go func() { results <- r.Close(t.Context()) }()
	}
	finish()
	<-returned
	for range 16 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if cleanups.Load() != 1 || response.Code != 204 {
		t.Fatal("cleanup not exactly once after actual completion")
	}
	if err := r.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentEntryAndClose(t *testing.T) {
	var cleanups atomic.Int64
	r := newRuntime(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }), func() error { cleanups.Add(1); return ErrClose })
	start := make(chan struct{})
	var wait sync.WaitGroup
	for range 64 {
		wait.Go(func() {
			<-start
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "https://oj.invalid", nil))
			if w.Code != 204 && w.Code != 503 {
				t.Error("unexpected racing response")
			}
		})
	}
	for range 8 {
		wait.Go(func() {
			<-start
			if !errors.Is(r.Close(t.Context()), ErrClose) {
				t.Error("close result was not stable")
			}
		})
	}
	close(start)
	wait.Wait()
	if cleanups.Load() != 1 {
		t.Fatal("concurrent close repeated cleanup")
	}
}

func TestPanicStillDrainsAndZeroValueFailsClosed(t *testing.T) {
	r := newRuntime(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("synthetic panic") }), func() error { return nil })
	func() {
		defer func() {
			if recover() == nil {
				t.Error("runtime unexpectedly swallowed handler panic")
			}
		}()
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "https://oj.invalid", nil))
	}()
	if err := r.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, zero := range []*Runtime{nil, {}} {
		w := httptest.NewRecorder()
		zero.ServeHTTP(w, httptest.NewRequest("GET", "https://oj.invalid", nil))
		if w.Code != 503 || !errors.Is(zero.Close(t.Context()), ErrConfig) {
			t.Fatal("zero runtime did not fail closed")
		}
	}
}

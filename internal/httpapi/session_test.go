package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
)

func TestSessionRoutesDelegateExactPathAndMetadata(t *testing.T) {
	for _, path := range []string{"/api/v1/session", "/api/v1/session/activity", "/api/v1/session/logout", "/api/v1/session/login/begin", "/api/v1/session/login/complete", "/api/v1/session/login/abandon"} {
		for _, method := range []string{"GET", "POST", "HEAD", "OPTIONS", "CUSTOM"} {
			r := httptest.NewRequest(method, "https://session.invalid"+path, nil)
			r.RemoteAddr = "192.0.2.1:1234"
			called := false
			h := httpapi.NewSessionHandler(http.HandlerFunc(func(w http.ResponseWriter, got *http.Request) {
				called = true
				if got != r || got.Host != "session.invalid" || got.TLS != r.TLS || got.RemoteAddr != "192.0.2.1:1234" {
					t.Error("session transport metadata rewritten")
				}
				w.WriteHeader(418)
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if !called || w.Code != 418 {
				t.Fatal("exact session method/path not delegated")
			}
		}
	}
	h := httpapi.NewSessionHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unknown or health path reached sessions") }))
	for _, path := range []string{"/api/v1/session/", "/api/v1/session/other", "/api/v1/session/login", "/api/v1/submissions"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatal("session prefix redirected or exposed an experiment route")
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/healthz", nil))
	if w.Code != 200 {
		t.Fatal("bootstrap health changed")
	}
}

func TestSessionPanicDoesNotPublishUncommittedCookie(t *testing.T) {
	h := httpapi.NewSessionHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Set-Cookie", "synthetic-token")
		panic("synthetic-secret")
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/session/login/complete", nil))
	if w.Code != 503 || w.Header().Get("Set-Cookie") != "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("panic published a cookie or omitted failure")
	}
	h = httpapi.NewSessionHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204); panic("synthetic-secret") }))
	func() {
		defer func() {
			if recover() != http.ErrAbortHandler {
				t.Error("committed panic was incorrectly converted to success/503")
			}
		}()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/v1/session/logout", nil))
	}()
}

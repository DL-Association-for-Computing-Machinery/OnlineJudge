package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
)

type readinessHandler struct {
	http.Handler
	ready func(context.Context) bool
}

func (h readinessHandler) Ready(ctx context.Context) bool       { return h.ready(ctx) }
func (h readinessHandler) AllowsReadiness(r *http.Request) bool { return r.Host == "oj.invalid" }

func TestSessionReadinessIsTLSOnlyAndDoesNotDiscloseDependencies(t *testing.T) {
	calls := 0
	h := httpapi.NewSessionHandler(readinessHandler{
		Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("probe reached ordinary authorization") }),
		ready: func(context.Context) bool {
			calls++
			if calls == 2 {
				panic("synthetic dependency secret")
			}
			return true
		},
	})
	const path = "/api/v1/readyz"
	for _, tc := range []struct {
		method, target, body string
		want                 int
	}{
		{"GET", "http://oj.invalid" + path, "", 404},
		{"GET", "https://oj.invalid" + path + "?", "", 404},
		{"GET", "https://oj.invalid/api/v1/%72eadyz", "", 404},
		{"GET", "https://oj.invalid" + path, "x", 404},
		{"GET", "https://other.invalid" + path, "", 404},
		{"POST", "https://oj.invalid" + path, "", 405},
		{"GET", "https://oj.invalid" + path, "", 200},
		{"GET", "https://oj.invalid" + path, "", 503},
	} {
		r := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" ||
			len(w.Header().Values("Set-Cookie")) != 0 || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("probe response for %s %s: %d %q", tc.method, tc.target, w.Code, w.Body.String())
		}
	}
	if calls != 2 {
		t.Fatal("invalid probe traffic reached dependencies")
	}
	w := httptest.NewRecorder()
	httpapi.NewHandler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	if w.Code != 404 {
		t.Fatal("bootstrap exposed readiness without dependencies")
	}
}

package httpapi

import (
	"net/http"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// NewSessionHandler delegates session and optional account/private-draft paths
// for every method without rewriting transport metadata. Runtime capabilities
// control availability. Health/unknown paths retain bootstrap; no redirects or
// experiment routes are added.
func NewSessionHandler(session http.Handler) http.Handler {
	bootstrap := NewHandler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		draft := path == "/api/v1/problem-drafts" || (strings.HasPrefix(path, "/api/v1/problem-drafts/") && identity.ValidID(strings.TrimSuffix(strings.TrimPrefix(path, "/api/v1/problem-drafts/"), "/preview")))
		switch r.URL.Path {
		case "/api/v1/session", "/api/v1/session/activity", "/api/v1/session/logout",
			"/api/v1/session/login/begin", "/api/v1/session/login/complete", "/api/v1/session/login/abandon", "/api/v1/account/password":
		default:
			if !draft {
				bootstrap.ServeHTTP(w, r)
				return
			}
		}
		if session == nil {
			sessionProblem(w)
			return
		}
		tracked := &sessionWriter{ResponseWriter: w}
		defer func() {
			if recover() != nil {
				if tracked.written {
					panic(http.ErrAbortHandler)
				}
				sessionProblem(w)
			}
		}()
		session.ServeHTTP(tracked, r)
	})
}

type sessionWriter struct {
	http.ResponseWriter
	written bool
}

func (w *sessionWriter) WriteHeader(status int) {
	w.written = true
	w.ResponseWriter.WriteHeader(status)
}
func (w *sessionWriter) Write(body []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(body)
}
func (w *sessionWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func sessionProblem(w http.ResponseWriter) {
	w.Header().Del("Set-Cookie")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"type":"about:blank","title":"会话服务暂时不可用","status":503,"code":"SESSION_UNAVAILABLE"}`))
}

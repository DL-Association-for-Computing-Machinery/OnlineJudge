package localdev

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

func localProblem(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "about:blank", "status": status, "code": code, "title": "本地开发入口不可用"})
}

func handler(api http.Handler, a account, site string) http.Handler {
	files := http.FileServer(http.Dir(site))
	protection := new(http.CrossOriginProtection)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.TLS == nil || r.Host != "localhost:8443" {
			localProblem(w, 403, "ORIGIN_REJECTED")
			return
		}
		if r.URL.Path == "/__local/login" {
			if r.Method != http.MethodPost {
				localProblem(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
			origins := r.Header.Values("Origin")
			sites := r.Header.Values("Sec-Fetch-Site")
			if protection.Check(r) != nil || len(origins) != 1 || origins[0] != Origin || (len(sites) > 0 && (len(sites) != 1 || sites[0] != "same-origin")) {
				localProblem(w, 403, "ORIGIN_REJECTED")
				return
			}
			b, err := io.ReadAll(io.LimitReader(r.Body, 1))
			if err != nil || len(b) != 0 {
				localProblem(w, 400, "INVALID_LOGIN")
				return
			}
			body, _ := json.Marshal(map[string]string{"email": a.Email, "password": a.Password})
			defer clear(body)
			request := r.Clone(r.Context())
			request.URL.Path = "/api/v1/session/login/begin"
			request.URL.RawPath = ""
			request.URL.RawQuery = ""
			request.RequestURI = request.URL.Path
			request.Body = io.NopCloser(bytes.NewReader(body))
			request.ContentLength = int64(len(body))
			request.TransferEncoding = nil
			request.Header.Set("Content-Type", "application/json")
			// Preserve actual TLS, browser source, Origin and cookies. The ordinary
			// Begin owns pending/active checks, KDF and quotas; browser does Complete.
			api.ServeHTTP(w, request)
			return
		}
		if r.URL.Path == "/__local/healthz" {
			if r.Method != http.MethodGet {
				localProblem(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/api/v1/healthz" {
			if r.Method != http.MethodGet {
				localProblem(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"ok","service":"api","scope":"process"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			api.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/__local/") {
			localProblem(w, 404, "NOT_FOUND")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			localProblem(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			files.ServeHTTP(w, r)
			return
		}
		// Serve only public build inputs, never the state directory or arbitrary files.
		http.ServeFile(w, r, filepath.Join(site, "index.html"))
	})
}

// Serve starts the separate local HTTPS listener and drains its real runtime.
func Serve(ctx context.Context, dir, site string) error {
	if err := validate(ctx, dir); err != nil {
		return err
	}
	c, err := configuration(dir)
	if err != nil {
		return errState
	}
	a, err := readAccount(dir)
	if err != nil {
		return err
	}
	cert, err := certificate(dir)
	if err != nil {
		return err
	}
	r, err := sessionruntime.Open(ctx, c)
	if err != nil {
		return errState
	}
	server := &http.Server{Addr: ":8443", Handler: handler(r, a, site), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}, ErrorLog: log.New(io.Discard, "", 0)}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServeTLS("", "") }()
	var serveErr error
	consumed := false
	select {
	case serveErr = <-result:
		consumed = true
	case <-ctx.Done():
	}
	stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	closeErr := stopServer(stop, server, r)
	if !consumed {
		serveErr = <-result
	}
	if closeErr != nil || (serveErr != nil && serveErr != http.ErrServerClosed) {
		return errState
	}
	return nil
}

type runtimeCloser interface{ Close(context.Context) error }

// Stop both admission paths together; a handler can need runtime drain to
// finish. All waits share one deadline and any incomplete drain is a failure.
func stopServer(ctx context.Context, server *http.Server, r runtimeCloser) error {
	closed := make(chan error, 1)
	go func() { closed <- r.Close(ctx) }()
	shutdownErr := server.Shutdown(ctx)
	if shutdownErr != nil {
		_ = server.Close()
	}
	return errors.Join(shutdownErr, <-closed)
}

// StateDirectory selects an explicit local volume path, never a remote endpoint.
func StateDirectory() string {
	if value := os.Getenv("OJ_LOCAL_STATE_DIR"); value != "" {
		return value
	}
	return "/state"
}

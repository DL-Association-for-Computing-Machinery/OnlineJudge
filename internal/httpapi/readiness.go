package httpapi

import (
	"context"
	"io"
	"net/http"
)

type sessionReadiness interface {
	AllowsReadiness(*http.Request) bool
	Ready(context.Context) bool
}

func serveSessionReadiness(w http.ResponseWriter, r *http.Request, probe sessionReadiness) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.TLS == nil || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if !probe.AllowsReadiness(r) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	ready := false
	func() {
		// A dependency failure must never spill its cause into the response.
		defer func() { _ = recover() }()
		ready = probe.Ready(r.Context())
	}()
	w.Header().Set("Content-Type", "application/json")
	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"status":"unavailable","service":"api","scope":"ordinary-session"}`)
		return
	}
	_, _ = io.WriteString(w, `{"status":"ready","service":"api","scope":"ordinary-session"}`)
}

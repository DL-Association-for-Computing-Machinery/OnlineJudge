package workerhost

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

var filePath = regexp.MustCompile(`^/file/[A-Za-z0-9_-]{1,128}$`)

// A 64 KiB source can expand sixfold under JSON escaping; include command fields.
const requestLimit = 512 * 1024

// RoundTrip sends only the bounded go-judge API subset through container-local
// curl. No host port, proxy, redirect, arbitrary URL or host file is accepted.
// Request bodies must be finite, in-memory readers as supplied by execution.GoJudge.
func (h *Host) RoundTrip(r *http.Request) (*http.Response, error) {
	if r == nil {
		return nil, errors.New("missing sandbox request")
	}
	if r.Body != nil {
		// Cleanup does not change the completed operation or its primary error.
		defer func() { _ = r.Body.Close() }()
	}
	if r.URL == nil || r.URL.Scheme != "http" || r.URL.Host != "127.0.0.1:5050" ||
		r.URL.User != nil || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Fragment != "" ||
		r.URL.Opaque != "" || r.URL.RawPath != "" || !allowed(r.Method, r.URL.Path) {
		return nil, errors.New("unsupported sandbox request")
	}
	ctx, done, err := h.begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer done()
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	var body []byte
	if r.Body != nil {
		body, err = io.ReadAll(io.LimitReader(r.Body, requestLimit+1))
		if err != nil || len(body) > requestLimit {
			return nil, errors.New("sandbox request exceeds budget")
		}
	}
	if len(body) != 0 && r.Method != http.MethodPost {
		return nil, errors.New("unexpected sandbox body")
	}
	// Recheck process lifetime immediately before dispatch, including cleanup.
	s, err := h.inspect(ctx, h.id)
	if err != nil {
		return nil, err
	}
	if err := h.validate(s, experiment.PinnedEnvironment()); err != nil {
		return nil, err
	}
	args := []string{"exec", "-i", h.id, "curl", "--disable", "--silent", "--show-error", "--noproxy", "*", "--max-time", "20", "--max-filesize", "131072", "--proto", "=http", "--request", r.Method, "--header", "Content-Type: application/json", "--write-out", "\n%{http_code}"}
	if r.Method == http.MethodPost {
		args = append(args, "--data-binary", "@-")
	}
	args = append(args, r.URL.String())
	out, err := h.docker(ctx, bytes.NewReader(body), args...)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Preserve actual status. curl's exit success alone is not HTTP success.
	i := bytes.LastIndexByte(out, '\n')
	if i < 0 || string(out[i+1:]) != "200" {
		return nil, errors.New("sandbox HTTP response rejected")
	}
	if i > 128*1024 {
		return nil, errors.New("sandbox response exceeds budget")
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(out[:i])), Request: r}, nil
}

func allowed(method, path string) bool {
	return method == http.MethodGet && (path == "/version" || path == "/config") ||
		method == http.MethodPost && path == "/run" || method == http.MethodDelete && filePath.MatchString(path)
}

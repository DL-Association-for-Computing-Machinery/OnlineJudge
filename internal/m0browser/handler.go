package m0browser

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/jsoninput"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

//go:embed assets/*
var assets embed.FS

const (
	nonceHeader = "X-OJ-M0-Nonce"
	subject     = "m0-browser-operator"
	scope       = "m0-browser-training"
)

type preset struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Source string `json:"source"`
}

// Fixed sources are the entire execution capability of this local page.
// The browser never selects a source, identity, language or material version.
var presets = [...]preset{
	{ID: "sum", Title: "正确求和示例", Source: "#include <cstdio>\n\nint main() {\n    int a, b;\n    scanf(\"%d%d\", &a, &b);\n    printf(\"%d\", a + b);\n}"},
	{ID: "wrong", Title: "错误答案示例", Source: "#include <cstdio>\n\nint main() {\n    puts(\"wrong\");\n}"},
}

type handler struct {
	store  submission.Store
	host   string
	origin string
	nonce  string
	guard  http.CrossOriginProtection
}

// NewHandler adds only /m0 routes to base, for a trusted local synthetic DB.
// The nonce is CSRF protection, not authentication of a person or account.
func NewHandler(base http.Handler, store submission.Store, addr string) (http.Handler, error) {
	if base == nil || store == nil || !localAddress(addr, false) {
		return nil, ErrConfiguration
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, ErrConfiguration
	}
	h := &handler{store: store, host: addr, origin: "http://" + addr, nonce: hex.EncodeToString(secret[:])}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/m0" || strings.HasPrefix(r.URL.Path, "/m0/") {
			h.ServeHTTP(w, r)
			return
		}
		base.ServeHTTP(w, r)
	}), nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'; worker-src 'none'")
	if r.Host != h.host || r.URL.RawQuery != "" || r.URL.Host != "" {
		writeProblem(w, 403, "ORIGIN_REJECTED")
		return
	}
	if name, ok := map[string]string{"/m0": "index.html", "/m0/": "index.html", "/m0/assets/page.js": "page.js", "/m0/assets/page.css": "page.css"}[r.URL.Path]; ok {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeProblem(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		data, err := assets.ReadFile("assets/" + name)
		if err != nil {
			writeProblem(w, 503, "PAGE_UNAVAILABLE")
			return
		}
		mediaType := map[string]string{"index.html": "text/html; charset=utf-8", "page.js": "text/javascript; charset=utf-8", "page.css": "text/css; charset=utf-8"}[name]
		w.Header().Set("Content-Type", mediaType)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
		return
	}
	method, known := map[string]string{"/m0/api/bootstrap": http.MethodGet, "/m0/api/submissions": http.MethodPost, "/m0/api/receipt": http.MethodGet}[r.URL.Path]
	if !known {
		writeProblem(w, 404, "NOT_FOUND")
		return
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		writeProblem(w, 405, "METHOD_NOT_ALLOWED")
		return
	}
	if r.URL.Path == "/m0/api/bootstrap" {
		writeJSON(w, 200, struct {
			Nonce   string   `json:"nonce"`
			Version string   `json:"version"`
			Presets []preset `json:"presets"`
		}{h.nonce, experiment.PinnedVersion, presets[:]})
		return
	}
	values := r.Header.Values(nonceHeader)
	if len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), []byte(h.nonce)) != 1 {
		writeProblem(w, 403, "NONCE_REJECTED")
		return
	}
	origins, sites := r.Header.Values("Origin"), r.Header.Values("Sec-Fetch-Site")
	if (len(origins) > 0 && (len(origins) != 1 || origins[0] != h.origin)) ||
		(r.Method == http.MethodPost && len(origins) != 1) ||
		(len(sites) > 0 && (len(sites) != 1 || sites[0] != "same-origin")) || h.guard.Check(r) != nil {
		writeProblem(w, 403, "ORIGIN_REJECTED")
		return
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !submission.ValidKey(keys[0]) {
		writeProblem(w, 400, "INVALID_SUBMISSION")
		return
	}
	if r.Method == http.MethodGet {
		result, err := h.store.Receipt(r.Context(), subject, scope, keys[0])
		h.respond(w, result, false, err)
		return
	}
	h.accept(w, r, keys[0])
}

func (h *handler) accept(w http.ResponseWriter, r *http.Request, key string) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeProblem(w, 415, "UNSUPPORTED_MEDIA_TYPE")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 512)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProblem(w, 413, "BODY_TOO_LARGE")
		} else {
			writeProblem(w, 400, "INVALID_SUBMISSION")
		}
		return
	}
	id, valid := decodePreset(body)
	if !valid {
		writeProblem(w, 400, "INVALID_SUBMISSION")
		return
	}
	for _, p := range presets {
		if p.ID == id {
			result, created, err := h.store.Accept(r.Context(), subject, scope, key, submission.Request{MaterialVersion: experiment.PinnedVersion, Language: "gnu++17", Source: p.Source})
			h.respond(w, result, created, err)
			return
		}
	}
	writeProblem(w, 400, "INVALID_SUBMISSION")
}

func decodePreset(body []byte) (string, bool) {
	if !jsoninput.ValidUnicode(body) {
		return "", false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') || !decoder.More() {
		return "", false
	}
	field, err := decoder.Token()
	if err != nil || field != "preset" {
		return "", false
	}
	var id string
	if decoder.Decode(&id) != nil || decoder.More() {
		return "", false // Also rejects duplicate fields instead of silently choosing one.
	}
	closing, err := decoder.Token()
	var trailing any
	return id, err == nil && closing == json.Delim('}') && decoder.Decode(&trailing) == io.EOF
}

func (*handler) respond(w http.ResponseWriter, result submission.Receipt, created bool, err error) {
	if err != nil {
		switch {
		case errors.Is(err, submission.ErrInvalid):
			writeProblem(w, 400, "INVALID_SUBMISSION")
		case errors.Is(err, submission.ErrConflict):
			writeProblem(w, 409, "IDEMPOTENCY_CONFLICT")
		case errors.Is(err, submission.ErrNotFound):
			writeProblem(w, 404, "NOT_FOUND")
		case errors.Is(err, submission.ErrMaterial):
			writeProblem(w, 503, "MATERIAL_UNAVAILABLE")
		default:
			writeProblem(w, 503, "ACCEPTANCE_UNKNOWN")
		}
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	writeJSON(w, status, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "about:blank", "status": status, "code": code, "title": "本机实验请求未完成"})
}

// Package httptransport adapts existing ordinary sessions to HTTPS. It neither
// admits sessions nor grants email, role, training, or resource permissions.
package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// CookieName is reserved for host-only, secure ordinary session credentials.
const CookieName = "__Host-oj_session"

// Store must provide fresh authority checks, explicit renewal and confirmed
// revocation, as implemented by redisstore.Store. Calls never imply a retry.
type Store interface {
	Read(context.Context, redisstore.Credential) (redisstore.View, error)
	Touch(context.Context, redisstore.Credential) (redisstore.View, error)
	Revoke(context.Context, redisstore.Credential) error
}

// Config freezes the trusted public HTTPS origin and a cooperative call budget.
// The adapter requires direct TLS; forwarded headers cannot establish trust.
type Config struct {
	Origin  string
	Timeout time.Duration
}

type handler struct {
	store      Store
	config     Config
	host       string
	protection http.CrossOriginProtection
}

// New builds three explicit endpoints without registering any application route:
// GET /api/v1/session, POST /api/v1/session/activity and POST /api/v1/session/logout.
// A future proxy integration must separately establish its TLS/host trust model.
func New(store Store, config Config) (http.Handler, error) {
	u, err := url.Parse(config.Origin)
	if store == nil || err != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		u.Opaque != "" || config.Origin != "https://"+u.Host || config.Timeout <= 0 || config.Timeout > 5*time.Second {
		return nil, errors.New("invalid session HTTP configuration")
	}
	return &handler{store: store, config: config, host: u.Host}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.TLS == nil || r.Host != h.host {
		problem(w, http.StatusForbidden, "ORIGIN_REJECTED", "请求来源不允许")
		return
	}
	method := ""
	switch r.URL.Path {
	case "/api/v1/session":
		method = http.MethodGet
	case "/api/v1/session/activity", "/api/v1/session/logout":
		method = http.MethodPost
	default:
		problem(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
		return
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		problem(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "请求方法不支持")
		return
	}
	if method == http.MethodPost && !h.sameOrigin(r) {
		problem(w, http.StatusForbidden, "ORIGIN_REJECTED", "请求来源不允许")
		return
	}
	credential, err := requestCredential(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "SESSION_REQUIRED", "会话无效或已过期")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.config.Timeout)
	defer cancel()
	var view redisstore.View
	switch r.URL.Path {
	case "/api/v1/session":
		view, err = h.store.Read(ctx, credential)
	case "/api/v1/session/activity":
		view, err = h.store.Touch(ctx, credential)
	case "/api/v1/session/logout":
		err = h.store.Revoke(ctx, credential)
	}
	// Cancellation does not prove rollback, including after a nil Store result.
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		sessionError(w, err)
		return
	}
	if r.URL.Path == "/api/v1/session/logout" {
		http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", Secure: true,
			HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0).UTC()})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Internal auth/recovery generations never cross the transport boundary.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		SubjectID        string    `json:"subject_id"`
		CreatedAt        time.Time `json:"created_at"`
		IdleExpiresAt    time.Time `json:"idle_expires_at"`
		AbsoluteDeadline time.Time `json:"absolute_deadline"`
	}{view.SubjectID, view.CreatedAt, view.IdleExpiresAt, view.AbsoluteDeadline})
}

func sessionError(w http.ResponseWriter, err error) {
	status, code, title := http.StatusServiceUnavailable, "SESSION_UNAVAILABLE", "会话服务暂时不可用"
	// Unknown effects always win. Explicit protocol refusal may deny identity
	// even inside a guard, but never confirms logout or rollback of an effect.
	failed := errors.Is(err, identity.ErrStore) || errors.Is(err, redisstore.ErrStore) ||
		errors.Is(err, identity.ErrRecovering) || errors.Is(err, redisstore.ErrUnknown) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	guarded := errors.Is(err, identity.ErrGuardedOperation)
	refused := errors.Is(err, redisstore.ErrStale) || errors.Is(err, redisstore.ErrNotFound)
	if !failed && (refused || (!guarded && (errors.Is(err, identity.ErrConflict) || errors.Is(err, identity.ErrNotFound)))) {
		status, code, title = http.StatusUnauthorized, "SESSION_REQUIRED", "会话无效或已过期"
	}
	problem(w, status, code, title)
}

func problem(w http.ResponseWriter, status int, code, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Status int    `json:"status"`
		Code   string `json:"code"`
	}{"about:blank", title, status, code})
}

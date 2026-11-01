package httptransport

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// AttemptCookieName contains an encrypted, short-lived pending login candidate.
// It is neither a session cookie nor a reusable password-verification grant.
const AttemptCookieName = "__Host-oj_login"

// NewLogin adds begin/complete/abandon POSTs to the existing HTTPS lifecycle.
// The same manager owns admission and existing sessions. It registers no default
// application routes or production parameters. Origin must match sealed context.
func NewLogin(login *redisstore.Login, config Config) (http.Handler, error) {
	if login == nil || login.Origin() != config.Origin {
		return nil, redisstore.ErrInvalid
	}
	base, err := New(login, config)
	if err != nil {
		return nil, err
	}
	h := base.(*handler)
	h.login = login
	return h, nil
}

// NewLimitedLogin requires an admission-configured manager. NewLogin also
// automatically enforces admission whenever its manager is configured; callers
// cannot accidentally bypass that configuration by using the older constructor.
// An optional password capability must belong to this exact Login. Omission
// preserves the original six endpoints and grants no password-change route.
func NewLimitedLogin(login *redisstore.Login, config Config, change ...*redisstore.PasswordChange) (http.Handler, error) {
	if !login.RateLimited() {
		return nil, redisstore.ErrInvalid
	}
	if len(change) > 1 || (len(change) == 1 && !change[0].ForLogin(login)) {
		return nil, redisstore.ErrInvalid
	}
	base, err := NewLogin(login, config)
	if err != nil {
		return nil, err
	}
	if len(change) == 1 {
		base.(*handler).passwordChange = change[0]
	}
	return base, nil
}

func (h *handler) serveLogin(w http.ResponseWriter, r *http.Request, source netip.Addr) {
	ctx, cancel := context.WithTimeout(r.Context(), h.config.Timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		loginError(w, err)
		return
	}
	raw, present, err := cookieValue(r, AttemptCookieName)
	if err != nil {
		loginError(w, redisstore.ErrInvalid)
		return
	}
	session, hasSession, err := cookieValue(r, CookieName)
	if err != nil {
		loginError(w, err)
		return
	}
	if hasSession && r.URL.Path != "/api/v1/session/login/abandon" {
		if _, err := redisstore.ParseCredential(session); err != nil {
			loginError(w, err)
			return
		}
	}
	if r.URL.Path == "/api/v1/session/login/begin" {
		if present {
			problem(w, 409, "LOGIN_PENDING", "请先确认或放弃当前登录尝试")
			return
		}
		h.beginLogin(w, r, ctx, source)
		return
	}
	if !present {
		loginError(w, redisstore.ErrInvalid)
		return
	}
	c, err := redisstore.ParseContinuation(raw)
	if err != nil {
		loginError(w, err)
		return
	}
	if !h.admitLogin(w, r, ctx, identity.Email{}, true, source) {
		return
	}
	if r.URL.Path == "/api/v1/session/login/abandon" {
		err = h.login.Abandon(ctx, c)
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			loginError(w, err)
			return
		}
		http.SetCookie(w, clearAttemptCookie())
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !h.canLogin(w, r, ctx) {
		return
	}
	credential, err := h.login.Complete(ctx, c)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		loginError(w, err)
		return
	}
	// Both headers occur only after Retry and a separate successful Check.
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: credential.Value(), Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.SetCookie(w, clearAttemptCookie())
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) canLogin(w http.ResponseWriter, r *http.Request, ctx context.Context) bool {
	raw, present, err := cookieValue(r, CookieName)
	if err != nil {
		loginError(w, err)
		return false
	}
	if !present {
		return true
	}
	c, err := redisstore.ParseCredential(raw)
	if err != nil {
		loginError(w, err)
		return false
	}
	_, err = h.store.Read(ctx, c)
	if err == nil {
		problem(w, 409, "ALREADY_AUTHENTICATED", "请先退出当前会话")
		return false
	}
	if sessionDenied(err) {
		return true
	}
	loginError(w, err)
	return false
}

func (h *handler) beginLogin(w http.ResponseWriter, r *http.Request, ctx context.Context, source netip.Addr) {
	email, secret, err := loginBody(w, r)
	if err != nil {
		return
	}
	defer clear(secret)
	if !h.admitLogin(w, r, ctx, email, false, source) {
		return
	}
	if !h.canLogin(w, r, ctx) {
		return
	}
	c, err := h.login.Prepare(ctx, email, secret)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		loginError(w, err)
		return
	}
	// Cookie lifetime is the original remaining window, never a new full window.
	remaining := time.Until(c.Deadline())
	if remaining <= 0 {
		loginError(w, redisstore.ErrStale)
		return
	}
	seconds := int((remaining + time.Second - 1) / time.Second)
	http.SetCookie(w, &http.Cookie{Name: AttemptCookieName, Value: c.Value(), Path: "/", Secure: true, HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Expires: c.Deadline().UTC(), MaxAge: seconds})
	w.WriteHeader(http.StatusAccepted)
}

func clearAttemptCookie() *http.Cookie {
	return &http.Cookie{Name: AttemptCookieName, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		MaxAge: -1, Expires: time.Unix(1, 0).UTC()}
}

func loginError(w http.ResponseWriter, err error) {
	// Unknown writes/commit outcomes must win over joined deterministic causes.
	unknown := errors.Is(err, redisstore.ErrStore) || errors.Is(err, identity.ErrStore) || errors.Is(err, identity.ErrRecovering) ||
		errors.Is(err, redisstore.ErrUnknown) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	status, code, title := 503, "LOGIN_UNAVAILABLE", "登录服务暂时不可用，请保留原尝试"
	if !unknown {
		switch {
		case errors.Is(err, password.ErrPassword):
			status, code, title = 400, "INVALID_LOGIN", "密码长度无效"
		case errors.Is(err, redisstore.ErrInvalid):
			status, code, title = 400, "LOGIN_ATTEMPT_INVALID", "登录尝试无效"
		case errors.Is(err, redisstore.ErrLimit):
			status, code, title = 409, "SESSION_LIMIT", "会话数量已达上限"
		case errors.Is(err, redisstore.ErrLoginRate):
			status, code, title = 429, "LOGIN_RATE_LIMITED", "登录请求过于频繁，请稍后重试"
		case errors.Is(err, redisstore.ErrConflict):
			status, code, title = 409, "LOGIN_ATTEMPT_CONFLICT", "登录尝试已改变，请勿重新创建重试"
		case errors.Is(err, identity.ErrCredentials) || sessionDenied(err):
			status, code, title = 401, "LOGIN_REJECTED", "凭据无效或登录尝试已失效"
		}
	}
	problem(w, status, code, title)
}

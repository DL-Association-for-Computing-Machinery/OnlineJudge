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

func (h *handler) servePasswordChange(w http.ResponseWriter, r *http.Request, source netip.Addr) {
	ctx, cancel := context.WithTimeout(r.Context(), h.config.Timeout)
	defer cancel()
	credential, err := requestCredential(r)
	if err != nil {
		passwordChangeError(w, redisstore.ErrStale)
		return
	}
	if !source.IsValid() {
		source, err = loginSource(r.RemoteAddr)
		if err != nil {
			passwordChangeError(w, err)
			return
		}
	}
	current, replacement, ok := passwordChangeBody(w, r)
	if !ok {
		return
	}
	defer clear(current)
	defer clear(replacement)
	err = h.passwordChange.Change(ctx, credential, source, current, replacement)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		passwordChangeError(w, err)
		return
	}
	// PostgreSQL's committed generation change is authoritative even when Redis
	// still holds all prior records. Cookie cleanup is only browser hygiene.
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0).UTC()})
	http.SetCookie(w, clearAttemptCookie())
	w.WriteHeader(http.StatusNoContent)
}

func passwordChangeError(w http.ResponseWriter, err error) {
	unknown := errors.Is(err, identity.ErrStore) || errors.Is(err, redisstore.ErrStore) || errors.Is(err, identity.ErrRecovering) || errors.Is(err, redisstore.ErrUnknown) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	status, code, title := 503, "PASSWORD_CHANGE_UNAVAILABLE", "密码修改结果无法确认，请重新登录后核验"
	if !unknown {
		switch {
		case errors.Is(err, redisstore.ErrPasswordChangeRate):
			status, code, title = 429, "PASSWORD_CHANGE_RATE_LIMITED", "修改请求过于频繁，请稍后重试"
		case errors.Is(err, password.ErrWeakPassword):
			status, code, title = 400, "WEAK_PASSWORD", "请勿使用常见弱密码"
		case errors.Is(err, password.ErrNewPasswordLength):
			status, code, title = 400, "INVALID_NEW_PASSWORD", "密码长度不符合站点要求"
		case errors.Is(err, password.ErrPassword):
			status, code, title = 400, "INVALID_PASSWORD_INPUT", "密码输入长度不符合要求"
		case errors.Is(err, identity.ErrPasswordUnchanged):
			status, code, title = 400, "PASSWORD_UNCHANGED", "新密码不能与当前密码相同"
		case errors.Is(err, identity.ErrConflict):
			status, code, title = 409, "PASSWORD_CHANGE_CONFLICT", "账号状态已变化，请重新登录后操作"
		case errors.Is(err, identity.ErrCredentials) || sessionDenied(err):
			status, code, title = 401, "PASSWORD_CHANGE_REJECTED", "当前凭据无效，请重新登录或核对当前密码"
		case errors.Is(err, redisstore.ErrInvalid):
			status, code, title = 400, "INVALID_PASSWORD_CHANGE", "修改请求无效"
		}
	}
	problem(w, status, code, title)
}

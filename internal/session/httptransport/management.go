package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const (
	// ManagementCookieName is independent of the ordinary login credential.
	ManagementCookieName = "__Host-oj_management"
	managementSession    = "/api/v1/platform-admin/session"
	managementLogin      = managementSession + "/login"
	managementActivity   = managementSession + "/activity"
	managementLogout     = managementSession + "/logout"
)

// IsManagementSessionPath recognizes only the four explicit management routes.
func IsManagementSessionPath(path string) bool {
	switch path {
	case managementSession, managementLogin, managementActivity, managementLogout:
		return true
	}
	return false
}

// ManagementSessions is the independently authenticated management scope.
type ManagementSessions interface {
	Login(context.Context, identity.Email, []byte) (redisstore.Credential, error)
	Read(context.Context, redisstore.Credential) (redisstore.ManagementView, error)
	Touch(context.Context, redisstore.Credential) (redisstore.ManagementView, error)
	Revoke(context.Context, redisstore.Credential) error
}

// WithManagementSessions enables management routes using the shared login budget.
func WithManagementSessions(base http.Handler, sessions ManagementSessions) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || h.login == nil || !h.login.RateLimited() || sessions == nil || h.management != nil {
		return nil, errors.New("invalid management session configuration")
	}
	next := *h
	next.management = sessions
	return &next, nil
}

func (h *handler) serveManagement(w http.ResponseWriter, r *http.Request, source netip.Addr) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	if h.management == nil {
		problem(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
		return
	}
	method := http.MethodPost
	if r.URL.Path == managementSession {
		method = http.MethodGet
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
	ctx, cancel := context.WithTimeout(r.Context(), h.config.Timeout)
	defer cancel()
	if r.URL.Path == managementLogin {
		h.serveManagementLogin(w, r, ctx, source)
		return
	}
	value, present, err := cookieValue(r, ManagementCookieName)
	credential, parseErr := redisstore.ParseCredential(value)
	if err != nil || !present || parseErr != nil {
		managementError(w, identity.ErrPlatformAdminRequired)
		return
	}
	var view redisstore.ManagementView
	switch r.URL.Path {
	case managementSession:
		view, err = h.management.Read(ctx, credential)
	case managementActivity:
		view, err = h.management.Touch(ctx, credential)
	case managementLogout:
		err = h.management.Revoke(ctx, credential)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		managementError(w, err)
		return
	}
	if r.URL.Path == managementLogout {
		cookie := managementCookie("")
		cookie.MaxAge, cookie.Expires = -1, time.Unix(1, 0).UTC()
		http.SetCookie(w, cookie)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		SubjectID        string    `json:"subject_id"`
		AbsoluteDeadline time.Time `json:"absolute_deadline"`
		IdleExpiresAt    time.Time `json:"idle_expires_at"`
	}{view.Actor.SubjectID, view.AbsoluteDeadline, view.IdleExpiresAt})
}

func (h *handler) serveManagementLogin(w http.ResponseWriter, r *http.Request, ctx context.Context, source netip.Addr) {
	email, secret, _, err := readLoginBody(w, r, false)
	defer clear(secret)
	if err != nil || !h.admitLogin(w, r, ctx, email, false, source) {
		return
	}
	value, present, err := cookieValue(r, ManagementCookieName)
	if err != nil {
		problem(w, http.StatusBadRequest, "INVALID_MANAGEMENT_COOKIE", "管理 Cookie 格式无效")
		return
	}
	if present {
		current, err := redisstore.ParseCredential(value)
		if err == nil {
			_, err = h.management.Read(ctx, current)
			switch {
			case err == nil:
				problem(w, http.StatusConflict, "MANAGEMENT_SESSION_ACTIVE", "请先退出当前管理会话")
				return
			case !sessionDenied(err) && !errors.Is(err, identity.ErrPlatformAdminRequired):
				managementError(w, err)
				return
			}
		}
	}
	credential, err := h.management.Login(ctx, email, secret)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		managementError(w, err)
		return
	}
	http.SetCookie(w, managementCookie(credential.Value()))
	w.WriteHeader(http.StatusNoContent)
}

func managementCookie(value string) *http.Cookie {
	return &http.Cookie{Name: ManagementCookieName, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}
}

func managementError(w http.ResponseWriter, err error) {
	if sessionDenied(err) || errors.Is(err, identity.ErrPlatformAdminRequired) || errors.Is(err, identity.ErrCredentials) {
		problem(w, http.StatusUnauthorized, "MANAGEMENT_SESSION_REQUIRED", "管理登录无效或已过期")
		return
	}
	problem(w, http.StatusServiceUnavailable, "MANAGEMENT_UNAVAILABLE", "管理会话服务暂时不可用")
}

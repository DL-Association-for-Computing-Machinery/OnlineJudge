package httptransport

import (
	"context"
	"errors"
	"net/http"
	"net/netip"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// RotateSessionPath replaces the current ordinary session credential.
const RotateSessionPath = "/api/v1/session/rotate"

type rotateSession func(context.Context, redisstore.Credential) (redisstore.LoginGrant, error)

// WithRotation enables cookie rotation for the current personal session.
func WithRotation(base http.Handler, rotate func(context.Context, redisstore.Credential) (redisstore.LoginGrant, error)) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || h.login == nil || !h.login.RateLimited() || rotate == nil || h.rotate != nil {
		return nil, errors.New("invalid session rotation configuration")
	}
	next := *h
	next.rotate = rotate
	return &next, nil
}

func (h *handler) serveRotate(w http.ResponseWriter, r *http.Request, ctx context.Context, credential redisstore.Credential, source netip.Addr) {
	// Only an existing session can spend the shared recovery budget. Rotate
	// independently rechecks it; this read does not authorize the write.
	if _, err := h.store.Read(ctx, credential); err != nil {
		sessionError(w, err)
		return
	}
	if !h.admitLogin(r, ctx, identity.Email{}, true, source, func(_ string, err error) { loginError(w, err) }) {
		return
	}
	grant, err := h.rotate(ctx, credential)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		sessionError(w, err)
		return
	}
	setSessionCookie(w, grant)
	w.WriteHeader(http.StatusNoContent)
}

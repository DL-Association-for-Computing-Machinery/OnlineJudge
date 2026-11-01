package httptransport

import (
	"context"
	"net/http"
	"net/netip"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func (h *handler) admitLogin(w http.ResponseWriter, r *http.Request, ctx context.Context, email identity.Email, recovery bool) bool {
	if !h.login.RateLimited() {
		return true
	}
	// Direct TLS is already required. Only the actual socket peer is trusted;
	// Forwarded/X-Forwarded-For/X-Real-IP never select a bucket.
	source, err := loginSource(r.RemoteAddr)
	if err != nil {
		loginError(w, err)
		return false
	}
	if recovery {
		err = h.login.AdmitRecovery(ctx, source)
	} else {
		err = h.login.AdmitBegin(ctx, source, email)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		loginError(w, err)
		return false
	}
	return true
}

func loginSource(raw string) (netip.Addr, error) {
	source, err := netip.ParseAddrPort(raw)
	if err != nil || source.Addr().Zone() != "" || source.Port() == 0 {
		return netip.Addr{}, redisstore.ErrInvalid
	}
	return source.Addr().Unmap(), nil
}

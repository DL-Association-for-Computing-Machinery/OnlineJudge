package httptransport

import (
	"context"
	"net/http"
	"net/netip"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func (h *handler) admitLogin(r *http.Request, ctx context.Context, email identity.Email, recovery bool, source netip.Addr, fail func(string, error)) bool {
	if !h.login.RateLimited() {
		return true
	}
	// A proxy source exists only after per-request mTLS and header validation.
	// Direct mode continues to ignore all forwarding headers.
	var err error
	if !source.IsValid() {
		source, err = loginSource(r.RemoteAddr)
		if err != nil {
			fail("admission", err)
			return false
		}
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
		fail("admission", err)
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

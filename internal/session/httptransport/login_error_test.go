package httptransport

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func TestLoginUnknownErrorsWinBeforeCookiePublication(t *testing.T) {
	for _, unknown := range []error{identity.ErrStore, redisstore.ErrStore, identity.ErrRecovering, redisstore.ErrUnknown, context.Canceled, context.DeadlineExceeded} {
		for _, refused := range []error{redisstore.ErrStale, redisstore.ErrLimit, redisstore.ErrConflict, redisstore.ErrInvalid, password.ErrPassword, identity.ErrCredentials} {
			w := httptest.NewRecorder()
			loginError(w, errors.Join(unknown, identity.ErrGuardedOperation, refused))
			if w.Code != 503 || len(w.Header().Values("Set-Cookie")) != 0 {
				t.Fatal("uncertain login result published a cookie or deterministic refusal")
			}
		}
	}
}

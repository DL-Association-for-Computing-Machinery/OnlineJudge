package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/netip"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailconfirm"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func withEmailConfirmation(ctx context.Context, handler http.Handler, authority emailconfirm.Store, store *redisstore.Store,
	work *password.BoundedHasher, secrets *redisstore.LoginSecrets, c Config,
) (http.Handler, error) {
	admission, err := redisstore.NewEmailAdmission(ctx, store, secrets, *c.EmailLimits)
	if err != nil {
		return nil, err
	}
	confirm, err := emailconfirm.New(authority, work, c.EmailConfirmation, admission.AdmitConfirmation)
	if err != nil {
		return nil, err
	}
	return httptransport.WithEmailConfirmation(handler, func(ctx context.Context, source netip.Addr, purpose emailtoken.Purpose,
		token string, replacement []byte,
	) httptransport.EmailConfirmationResult {
		var err error
		switch purpose {
		case emailtoken.Activation:
			err = confirm.Activate(ctx, source, token, replacement)
		case emailtoken.Recovery:
			err = confirm.Reset(ctx, source, token, replacement)
		default:
			return httptransport.EmailConfirmationUnavailable
		}
		switch {
		case err == nil:
			return httptransport.EmailConfirmationCommitted
		case errors.Is(err, emailconfirm.ErrLink):
			return httptransport.EmailConfirmationInvalidLink
		case errors.Is(err, redisstore.ErrEmailRate):
			return httptransport.EmailConfirmationRateLimited
		case errors.Is(err, password.ErrWeakPassword):
			return httptransport.EmailConfirmationWeakPassword
		case errors.Is(err, password.ErrNewPasswordLength):
			return httptransport.EmailConfirmationInvalidPassword
		default:
			return httptransport.EmailConfirmationUnavailable
		}
	})
}

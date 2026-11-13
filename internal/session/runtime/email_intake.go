package runtime

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailbudget"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/emailintake"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// IsolatedEmailIntakeConfig supplies a caller-owned, independent budget pool
// and the immutable payload protector. Keep that pool open until Close succeeds.
type IsolatedEmailIntakeConfig struct {
	Limits    emailintake.Config
	Protector *emailtoken.Protector
	Ledger    *pgxpool.Pool
}

func withIsolatedEmailIntake(ctx context.Context, handler http.Handler, authority *identitypg.Store, store *redisstore.Store,
	work *password.BoundedHasher, secrets *redisstore.LoginSecrets, pool *pgxpool.Pool, c Config,
) (http.Handler, func(), error) {
	ic := c.IsolatedEmailIntake
	if c.EmailLimits == nil || c.EmailConfirmation == nil || ic.Ledger == nil ||
		ic.Ledger.Config().ConnConfig.Database == c.Database.Database {
		return nil, nil, ErrConfig
	}
	protector := ic.Protector
	budget, err := mailbudget.New(ic.Ledger, protector)
	if err != nil {
		return nil, nil, err
	}
	if err = budget.CheckReady(ctx, ic.Ledger.Config().ConnConfig.Database); err != nil {
		return nil, nil, err
	}
	queue, err := mailqueue.NewEnqueuer(ctx, pool)
	if err != nil {
		return nil, nil, err
	}
	admission, err := redisstore.NewEmailAdmission(ctx, store, secrets, *c.EmailLimits)
	if err != nil {
		return nil, nil, err
	}
	intake, err := emailintake.New(ic.Limits, authority.RecoveryState, admission.AdmitRequest,
		func(ctx context.Context, address identity.Email, purpose emailtoken.Purpose, recoveryID string) {
			// The acknowledgement is already returned. Never repeat unknown
			// issuance or send address-bearing diagnostics from this callback.
			_ = authority.ProcessEmailRequest(ctx, address, purpose, recoveryID, work, protector, budget.Reserve, queue.Enqueue)
		})
	if err != nil {
		return nil, nil, err
	}
	next, err := httptransport.WithEmailRequest(handler, intake.Submit)
	if err != nil {
		intake.Drain()
		return nil, nil, err
	}
	return next, intake.Drain, nil
}

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

// EmailIntakeConfig selects a private payload key file and a dedicated budget
// database. The runtime owns its separate pool and the bounded intake workers.
type EmailIntakeConfig struct {
	Limits         emailintake.Config
	PayloadKeyFile string
	BudgetDatabase DatabaseConfig
}

func withEmailIntake(ctx context.Context, handler http.Handler, authority *identitypg.Store, store *redisstore.Store,
	work *password.BoundedHasher, secrets *redisstore.LoginSecrets, pool *pgxpool.Pool, c Config,
) (http.Handler, func(), error) {
	ic := c.EmailIntake
	if c.EmailLimits == nil || c.EmailConfirmation == nil || ic.Limits.Validate() != nil ||
		ic.BudgetDatabase.Database == c.Database.Database || ic.PayloadKeyFile == c.SecretsFile {
		return nil, nil, ErrConfig
	}
	pgConfig, err := databaseConfig(ic.BudgetDatabase)
	if err != nil {
		return nil, nil, err
	}
	protector, err := emailtoken.LoadProtector(ic.PayloadKeyFile)
	if err != nil {
		return nil, nil, err
	}
	ledger, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		return nil, nil, err
	}
	keepLedger := false
	defer func() {
		if !keepLedger {
			ledger.Close()
		}
	}()
	budget, err := mailbudget.New(ledger, protector)
	if err != nil {
		return nil, nil, err
	}
	if err = budget.CheckReady(ctx, ic.BudgetDatabase.Database); err != nil {
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
	keepLedger = true
	return next, func() {
		intake.Drain()
		ledger.Close()
	}, nil
}

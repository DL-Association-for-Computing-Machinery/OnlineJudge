package runtime

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

// Open owns the application pool, Redis client, identity authority and shared
// KDF admission, plus a separate budget pool when email intake is enabled.
// It only checks existing schema/recovery/limit metadata; it never
// migrates, provisions limits, rotates/opens recovery, starts a server or retries.
// On failure all created clients are closed. Error text contains no driver cause,
// credential, path or key; production parameters and TLS listener remain caller-owned.
func Open(ctx context.Context, c Config) (*Runtime, error) {
	if ctx == nil {
		return nil, ErrConfig
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.StartupTimeout <= 0 || c.StartupTimeout > time.Minute {
		return nil, ErrConfig
	}
	startup, cancel := context.WithTimeout(ctx, c.StartupTimeout)
	defer cancel()
	hasher, work, err := c.validate()
	if err != nil {
		return nil, err
	}
	pgConfig, err := databaseConfig(c.Database)
	if err != nil {
		return nil, err
	}
	rc := c.Redis
	rc.TLS, err = cloneTLS(c.RedisSecurity, rc.TLS)
	if err != nil {
		return nil, err
	}
	client, err := redisstore.NewClient(rc)
	if err != nil {
		return nil, ErrConfig
	}
	keep := false
	closeDrafts := func() error { return nil }
	defer func() {
		if !keep {
			_ = closeDrafts()
			_ = client.Close()
		}
	}()
	secrets, err := redisstore.LoadLoginSecrets(c.SecretsFile)
	if err != nil {
		return nil, ErrSecrets
	}
	if err := startup.Err(); err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(startup, pgConfig)
	if err != nil {
		return nil, ErrStartup
	}
	defer func() {
		if !keep {
			pool.Close()
		}
	}()
	if pool.Ping(startup) != nil || db.CheckSchema(startup, pool) != nil {
		return nil, startupFailure(startup)
	}
	authority, err := identitypg.New(pool, hasher)
	if err != nil {
		return nil, ErrConfig
	}
	state, err := authority.RecoveryState(startup)
	if err != nil || !state.Enabled || !identity.ValidRecoveryID(state.Generation) {
		return nil, startupFailure(startup)
	}
	store, err := redisstore.New(startup, client, authority, work, c.Namespace, redisstore.OrdinaryPolicy())
	if err != nil {
		return nil, startupFailure(startup)
	}
	login, err := redisstore.NewLimitedLogin(startup, store, c.HTTP.Origin, c.LoginWindow, secrets, c.LoginLimits)
	if err != nil {
		return nil, startupFailure(startup)
	}
	var handler http.Handler
	var change []*redisstore.PasswordChange
	if c.PasswordChange != nil {
		capability, err := redisstore.NewPasswordChange(startup, login, secrets, c.PasswordChange.Limits, c.PasswordChange.Acceptance)
		if err != nil {
			return nil, startupFailure(startup)
		}
		change = append(change, capability)
	}
	if c.Mode == DirectTLS {
		handler, err = httptransport.NewLimitedLogin(login, c.HTTP, change...)
	} else {
		handler, err = httptransport.NewProxiedLimitedLogin(login, c.HTTP, c.Proxy, change...)
	}
	if err != nil {
		return nil, ErrConfig
	}
	if c.PrivateDrafts != nil {
		drafts, cleanup, err := openDrafts(startup, *c.PrivateDrafts, store, pool, secrets)
		if err != nil {
			return nil, err
		}
		closeDrafts = cleanup
		handler, err = httptransport.WithPrivateDrafts(handler, drafts.importer, c.PrivateDrafts.Timeout)
		if err != nil {
			return nil, ErrConfig
		}
		if drafts.references != nil {
			handler, err = httptransport.WithReferenceRequests(handler, drafts.references, drafts.preparation)
			if err != nil {
				return nil, ErrConfig
			}
		}
		if drafts.review != nil {
			handler, err = httptransport.WithReviewIntake(handler, drafts.review, drafts.solutions)
			if err != nil {
				return nil, ErrConfig
			}
		}
		if drafts.evidence != nil {
			var returns httptransport.ReviewReturns
			var statements httptransport.StatementConfirmations
			if drafts.returns != nil {
				returns = drafts.returns
			}
			if drafts.statements != nil {
				statements = drafts.statements
			}
			handler, err = httptransport.WithReviewEvidence(handler, drafts.evidence, drafts.materials, returns, statements)
			if err != nil {
				return nil, ErrConfig
			}
			handler, err = httptransport.WithReviewDiscovery(handler, drafts.discovery)
			if err != nil {
				return nil, ErrConfig
			}
		}
	}
	if c.SessionRotation {
		handler, err = httptransport.WithRotation(handler, store.Rotate)
		if err != nil {
			return nil, ErrConfig
		}
	}
	if c.SessionDevices {
		handler, err = httptransport.WithDevices(handler, store)
		if err != nil {
			return nil, ErrConfig
		}
	}
	if err := startup.Err(); err != nil {
		return nil, err
	}
	if c.EmailConfirmation != nil {
		handler, err = withEmailConfirmation(startup, handler, authority, store, work, secrets, c)
		if err != nil {
			return nil, startupFailure(startup)
		}
	}
	if c.TrainingHistory {
		handler, err = httptransport.WithTrainingHistory(handler, func(ctx context.Context, actor identity.TrainingActor, after *submission.HistoryPosition) (submission.HistoryPage, error) {
			return submissionpg.ListTrainingHistory(ctx, pool, actor, after)
		})
		if err != nil {
			return nil, ErrConfig
		}
	}
	if c.TrainingSubmissionDetail {
		handler, err = httptransport.WithTrainingDetail(handler, func(ctx context.Context, actor identity.TrainingActor, id string) (submission.Detail, error) {
			return submissionpg.ReadTrainingDetail(ctx, pool, actor, id)
		}, func(ctx context.Context, actor identity.TrainingActor, id string) (submission.Receipt, error) {
			return submissionpg.ReadTrainingStatus(ctx, pool, actor, id)
		})
		if err != nil {
			return nil, ErrConfig
		}
	}
	closeIntake := func() {}
	if c.EmailIntake != nil {
		handler, closeIntake, err = withEmailIntake(startup, handler, authority, store, work, secrets, pool, c)
		if err != nil {
			return nil, startupFailure(startup)
		}
	}
	if err := startup.Err(); err != nil {
		closeIntake()
		return nil, err
	}
	result := newRuntime(handler, func() error {
		closeIntake()
		draftErr := closeDrafts()
		err := client.Close()
		pool.Close()
		if err != nil || draftErr != nil {
			return ErrClose
		}
		return nil
	})
	keep = true
	return result, nil
}

func startupFailure(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrStartup
}

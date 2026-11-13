package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// RecoveryAction explicitly selects one offline maintenance operation.
type RecoveryAction string

const (
	RecoveryStatus RecoveryAction = "status"
	RecoveryClose  RecoveryAction = "close"
	RecoveryOpen   RecoveryAction = "open"
)

var (
	// ErrRecoveryEpoch means that the requested closed epoch is no longer current.
	ErrRecoveryEpoch = errors.New("session recovery epoch is not the current closed epoch")
	// ErrRecoveryRedis means that the dedicated replacement Redis is unavailable,
	// incompatible, nonempty, or could not be provisioned and verified.
	ErrRecoveryRedis = errors.New("replacement Redis is not empty, compatible and ready")
)

// RecoveryRequest confirms the intended database and, for opening, the exact
// epoch and operator's latest account-fact review. The review is external.
type RecoveryRequest struct {
	Action           RecoveryAction
	Database         string
	Generation       string
	AccountsReviewed bool
}

// MaintainRecovery is for an externally isolated restored database and a
// dedicated replacement Redis. It never starts API processes or deletes data.
// A non-nil error can follow an uncertain write; inspect status before retrying.
func MaintainRecovery(ctx context.Context, c Config, req RecoveryRequest) (identity.RecoveryState, error) {
	if ctx == nil || req.Database == "" || req.Database != c.Database.Database {
		return identity.RecoveryState{}, ErrConfig
	}
	switch req.Action {
	case RecoveryStatus, RecoveryClose:
		if req.Generation != "" || req.AccountsReviewed {
			return identity.RecoveryState{}, ErrConfig
		}
	case RecoveryOpen:
		if !identity.ValidRecoveryID(req.Generation) || !req.AccountsReviewed {
			return identity.RecoveryState{}, ErrConfig
		}
	default:
		return identity.RecoveryState{}, ErrConfig
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	config, err := databaseConfig(c.Database)
	if err != nil {
		return identity.RecoveryState{}, err
	}
	hasher, err := password.New(c.PasswordPolicy, c.PasswordLimits)
	if err != nil {
		return identity.RecoveryState{}, ErrConfig
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return identity.RecoveryState{}, ErrStartup
	}
	defer pool.Close()
	var connected string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&connected); err != nil {
		return identity.RecoveryState{}, ErrStartup
	}
	if connected != req.Database {
		return identity.RecoveryState{}, ErrConfig
	}
	if err := db.CheckSchema(ctx, pool); err != nil {
		return identity.RecoveryState{}, ErrStartup
	}
	authority, err := identitypg.New(pool, hasher)
	if err != nil {
		return identity.RecoveryState{}, ErrConfig
	}
	switch req.Action {
	case RecoveryStatus:
		return authority.RecoveryState(ctx)
	case RecoveryClose:
		generation, err := authority.BeginRecovery(ctx)
		return identity.RecoveryState{Generation: generation}, err
	}
	state, err := authority.RecoveryState(ctx)
	if err != nil {
		return identity.RecoveryState{}, err
	}
	if state.Enabled || state.Generation != req.Generation {
		return identity.RecoveryState{}, ErrRecoveryEpoch
	}
	if err := prepareReplacementRedis(ctx, c, authority); err != nil {
		return identity.RecoveryState{}, err
	}
	if err := authority.FinishRecovery(ctx, req.Generation); err != nil {
		if errors.Is(err, identity.ErrConflict) {
			return identity.RecoveryState{}, ErrRecoveryEpoch
		}
		return identity.RecoveryState{}, err
	}
	return identity.RecoveryState{Generation: req.Generation, Enabled: true}, nil
}

func prepareReplacementRedis(ctx context.Context, c Config, authority *identitypg.Store) error {
	_, work, err := c.validate()
	if err != nil {
		return err
	}
	rc := c.Redis
	rc.TLS, err = cloneTLS(c.RedisSecurity, rc.TLS)
	if err != nil {
		return err
	}
	client, err := redisstore.NewClient(rc)
	if err != nil {
		return ErrConfig
	}
	defer func() { _ = client.Close() }()
	secrets, err := redisstore.LoadLoginSecrets(c.SecretsFile)
	if err != nil {
		return ErrSecrets
	}
	store, err := redisstore.New(ctx, client, authority, work, c.Namespace, redisstore.OrdinaryPolicy())
	if err != nil {
		return ErrRecoveryRedis
	}
	count, err := client.DBSize(ctx).Result()
	if err != nil || count != 0 {
		return ErrRecoveryRedis
	}
	if err := redisstore.InitializeLoginLimits(ctx, store, secrets, c.LoginLimits); err != nil {
		return ErrRecoveryRedis
	}
	login, err := redisstore.NewLimitedLogin(ctx, store, c.HTTP.Origin, c.LoginWindow, secrets, c.LoginLimits)
	if err != nil {
		return ErrRecoveryRedis
	}
	if change := c.PasswordChange; change != nil {
		if err := redisstore.InitializePasswordChangeLimits(ctx, store, secrets, change.Limits, change.Acceptance); err != nil {
			return ErrRecoveryRedis
		}
		if _, err := redisstore.NewPasswordChange(ctx, login, secrets, change.Limits, change.Acceptance); err != nil {
			return ErrRecoveryRedis
		}
	}
	if c.EmailLimits != nil {
		if err := redisstore.InitializeEmailLimits(ctx, store, secrets, *c.EmailLimits); err != nil {
			return ErrRecoveryRedis
		}
		if _, err := redisstore.NewEmailAdmission(ctx, store, secrets, *c.EmailLimits); err != nil {
			return ErrRecoveryRedis
		}
	}
	return nil
}

package apiapp

import (
	"context"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

// MaintainRecovery loads the same storage and password-policy material as the
// API without loading a listener certificate or starting any HTTP service.
func MaintainRecovery(ctx context.Context, path string, lookup Lookup, req sessionruntime.RecoveryRequest) (identity.RecoveryState, error) {
	if ctx == nil || lookup == nil {
		return identity.RecoveryState{}, ErrConfiguration
	}
	c, err := readConfig(ctx, path)
	if err != nil {
		return identity.RecoveryState{}, err
	}
	c.Runtime.Database.Password, err = password(c.DatabaseAuth, "OJ_SESSION_DATABASE_PASSWORD", lookup)
	if err != nil {
		return identity.RecoveryState{}, err
	}
	c.Runtime.Database.TLS, err = storageTLS(c.Runtime.Database.Security, c.DatabaseTLS)
	if err != nil {
		return identity.RecoveryState{}, err
	}
	if req.Action == sessionruntime.RecoveryOpen {
		c.Runtime.Redis.Password, err = password(c.RedisAuth, "OJ_SESSION_REDIS_PASSWORD", lookup)
		if err != nil {
			return identity.RecoveryState{}, err
		}
		c.Runtime.Redis.TLS, err = storageTLS(c.Runtime.RedisSecurity, c.RedisTLS)
		if err != nil {
			return identity.RecoveryState{}, err
		}
		if err := loadPasswordAcceptance(&c); err != nil {
			return identity.RecoveryState{}, err
		}
	}
	return sessionruntime.MaintainRecovery(ctx, c.Runtime, req)
}

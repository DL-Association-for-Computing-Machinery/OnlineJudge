package redisstore

import (
	"crypto/tls"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
)

// ClientConfig requires explicit per-client connection and timeout budgets.
// TLS/credentials and a private trusted endpoint are deployment responsibilities;
// this package neither logs configuration nor contacts an endpoint at construction.
type ClientConfig struct {
	Addr, Username, Password string
	TLS                      *tls.Config
	Connections              int
	Timeout                  time.Duration
}

// NewClient builds a caller-owned standalone client with no command retries,
// one dial attempt, and no independent pipeline pool or maintenance handoff pool.
// Share it across stores/scopes; close it only after their calls finish.
func NewClient(c ClientConfig) (*redis.Client, error) {
	if c.Addr == "" || c.Connections <= 0 || c.Connections > 1024 || c.Timeout <= 0 || c.Timeout > 5*time.Second {
		return nil, ErrInvalid
	}
	var tlsConfig *tls.Config
	if c.TLS != nil {
		tlsConfig = c.TLS.Clone()
	}
	return redis.NewClient(&redis.Options{
		Addr: c.Addr, Username: c.Username, Password: c.Password, TLSConfig: tlsConfig,
		Protocol: 2, MaxRetries: -1, DialerRetries: 1,
		DisableIdentity: true,
		DialTimeout:     c.Timeout, ReadTimeout: c.Timeout, WriteTimeout: c.Timeout, PoolTimeout: c.Timeout,
		PoolSize: c.Connections, MaxActiveConns: c.Connections, MaxConcurrentDials: c.Connections,
		PipelinePoolSize: -1, ContextTimeoutEnabled: true,
		MaintNotificationsConfig: &maintnotifications.Config{Mode: maintnotifications.ModeDisabled},
	}), nil
}

func boundedClient(client *redis.Client) bool {
	if client == nil {
		return false
	}
	o := client.Options()
	return o.MaxRetries == 0 && o.DialerRetries == 1 && o.Protocol == 2 && o.DisableIdentity &&
		o.ClientSideCache == nil && o.ClientSideCacheConfig == nil && o.MaintNotificationsConfig != nil && o.MaintNotificationsConfig.Mode == maintnotifications.ModeDisabled &&
		o.PoolSize > 0 && o.MaxActiveConns == o.PoolSize && o.MaxConcurrentDials > 0 && o.MaxConcurrentDials <= o.MaxActiveConns &&
		o.PipelinePoolSize < 0 && o.ContextTimeoutEnabled && o.DialTimeout > 0 && o.DialTimeout <= 5*time.Second &&
		o.ReadTimeout > 0 && o.ReadTimeout <= 5*time.Second && o.WriteTimeout > 0 && o.WriteTimeout <= 5*time.Second && o.PoolTimeout > 0 && o.PoolTimeout <= 5*time.Second
}

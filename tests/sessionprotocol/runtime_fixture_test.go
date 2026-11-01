//go:build integration

package sessionprotocol_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

func runtimeConfig(t *testing.T, f *fixture, ns string, provision bool) sessionruntime.Config {
	t.Helper()
	key, limiting := loginKey(t), loginKey(t)
	path := loginSecretsFile(t, key, limiting)
	limits := rateLimits()
	if provision {
		secrets, err := protocol.LoadLoginSecrets(path)
		must(t, err)
		must(t, protocol.InitializeLoginLimits(f.ctx, f.store(t, ns, 0, protocol.OrdinaryPolicy()), secrets, limits))
	}
	parsed, err := pgxpool.ParseConfig(os.Getenv("OJ_PROTOCOL_DATABASE_URL"))
	must(t, err)
	return sessionruntime.Config{
		Database: sessionruntime.DatabaseConfig{Host: parsed.ConnConfig.Host, Port: parsed.ConnConfig.Port, Database: parsed.ConnConfig.Database, Username: parsed.ConnConfig.User,
			ApplicationName: "runtime_" + ns, Security: sessionruntime.PrivateTransport, Connections: 2, Timeout: time.Second, MaxLifetime: time.Hour, MaxIdle: time.Minute, HealthCheck: time.Second},
		Redis: protocol.ClientConfig{Addr: os.Getenv("OJ_PROTOCOL_REDIS_ADDR"), Connections: 2, Timeout: time.Second}, RedisSecurity: sessionruntime.PrivateTransport,
		PasswordPolicy:    password.Policy{Current: password.Parameters{MemoryKiB: 128, Iterations: 2, Parallelism: 2, SaltBytes: 16, DigestBytes: 32}, Legacy: []password.Parameters{{MemoryKiB: 64, Iterations: 1, Parallelism: 1, SaltBytes: 8, DigestBytes: 24}}},
		PasswordLimits:    password.Limits{MaxMemoryKiB: 256, MaxIterations: 4, MaxParallelism: 4, MaxPasswordBytes: 128, MaxSaltBytes: 64, MaxDigestBytes: 64},
		PasswordAdmission: password.AdmissionLimits{MaxInFlight: 2, MaxWaiting: 4, MaxQueueWait: time.Second},
		Namespace:         ns, SecretsFile: path, LoginWindow: time.Minute, StartupTimeout: 5 * time.Second, LoginLimits: limits,
		HTTP: transport.Config{Origin: loginOrigin, Timeout: time.Second}, Mode: sessionruntime.DirectTLS,
	}
}

func redisConnections(t *testing.T, f *fixture) int {
	t.Helper()
	list, err := f.control.ClientList(t.Context()).Result()
	must(t, err)
	return len(strings.Split(strings.TrimSpace(list), "\n"))
}

func sameRedisConnections(t *testing.T, f *fixture, count int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for redisConnections(t, f) != count {
		select {
		case <-tick.C:
		case <-ctx.Done():
			t.Fatal("runtime leaked or closed another owner's Redis connections")
		}
	}
}

func openRuntime(t *testing.T, c sessionruntime.Config) *sessionruntime.Runtime {
	t.Helper()
	r, err := sessionruntime.Open(t.Context(), c)
	must(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		must(t, r.Close(ctx))
	})
	return r
}

func runtimeConnections(t *testing.T, f *fixture, label string) int {
	t.Helper()
	var n int
	must(t, f.pools[0].QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE application_name=$1`, label).Scan(&n))
	return n
}

func noRuntimeConnections(t *testing.T, f *fixture, label string) {
	t.Helper()
	deadline, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for runtimeConnections(t, f, label) != 0 {
		select {
		case <-tick.C:
		case <-deadline.Done():
			t.Fatal("failed/closed runtime retained PostgreSQL connections")
		}
	}
}

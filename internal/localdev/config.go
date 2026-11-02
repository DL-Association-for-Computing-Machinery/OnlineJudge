// Package localdev owns the isolated, loopback-published development stack.
// Production API binaries do not import it. Its defaults are not deployment budgets.
package localdev

import (
	"crypto/sha256"
	"path/filepath"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

// Origin is the only public origin accepted by this separate development server.
const Origin = "https://localhost:8443"

const databaseURL = "host=postgres port=5432 user=oj_local dbname=oj_local password='' passfile=/dev/null sslmode=disable connect_timeout=3"

func configuration(dir string) (sessionruntime.Config, error) {
	// A deliberately small development dictionary, not production coverage.
	rules, err := password.NewAcceptance(15, 256, [][32]byte{sha256.Sum256([]byte("passwordpassword")), sha256.Sum256([]byte("123456789012345"))})
	if err != nil {
		return sessionruntime.Config{}, err
	}
	bucket := redisstore.LoginBucket{Count: 120, Window: time.Hour}
	return sessionruntime.Config{
		Database: sessionruntime.DatabaseConfig{Host: "postgres", Port: 5432, Database: "oj_local", Username: "oj_local", ApplicationName: "oj_local_session", Security: sessionruntime.PrivateTransport, Connections: 4, Timeout: 3 * time.Second, MaxLifetime: time.Hour, MaxIdle: time.Minute, HealthCheck: 10 * time.Second},
		Redis:    redisstore.ClientConfig{Addr: "redis:6379", Connections: 4, Timeout: 3 * time.Second}, RedisSecurity: sessionruntime.PrivateTransport,
		PasswordPolicy:    password.Policy{Current: password.Parameters{MemoryKiB: 19456, Iterations: 2, Parallelism: 1, SaltBytes: 16, DigestBytes: 32}},
		PasswordLimits:    password.Limits{MaxMemoryKiB: 19456, MaxIterations: 2, MaxParallelism: 1, MaxPasswordBytes: 256, MaxSaltBytes: 16, MaxDigestBytes: 32},
		PasswordAdmission: password.AdmissionLimits{MaxInFlight: 2, MaxWaiting: 4, MaxQueueWait: 2 * time.Second},
		Namespace:         "local-development", SecretsFile: filepath.Join(dir, "login-keys.json"), LoginWindow: time.Minute, StartupTimeout: 15 * time.Second,
		LoginLimits: redisstore.LoginLimits{BeginGlobal: bucket, BeginSource: bucket, BeginEmail: bucket, RecoveryGlobal: bucket, RecoverySource: bucket},
		HTTP:        httptransport.Config{Origin: Origin, Timeout: 5 * time.Second}, Mode: sessionruntime.DirectTLS,
		PasswordChange: &sessionruntime.PasswordChangeConfig{Acceptance: rules, Limits: redisstore.PasswordChangeLimits{Global: bucket, Source: bucket, Subject: bucket}},
	}, nil
}

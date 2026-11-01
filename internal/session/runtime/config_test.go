package runtime

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func validConfig() Config {
	b := redisstore.LoginBucket{Count: 10, Window: time.Hour}
	return Config{
		Database: DatabaseConfig{Host: "127.0.0.1", Port: 5432, Database: "fixture", Username: "fixture", Password: "synthetic-secret", ApplicationName: "runtime-test", Security: PrivateTransport, Connections: 2, Timeout: time.Second, MaxLifetime: time.Hour, MaxIdle: time.Minute, HealthCheck: time.Second},
		Redis:    redisstore.ClientConfig{Addr: "127.0.0.1:1", Password: "synthetic-secret", Connections: 2, Timeout: time.Second}, RedisSecurity: PrivateTransport,
		PasswordPolicy:    password.Policy{Current: password.Parameters{MemoryKiB: 128, Iterations: 2, Parallelism: 2, SaltBytes: 16, DigestBytes: 32}},
		PasswordLimits:    password.Limits{MaxMemoryKiB: 256, MaxIterations: 4, MaxParallelism: 4, MaxPasswordBytes: 128, MaxSaltBytes: 64, MaxDigestBytes: 64},
		PasswordAdmission: password.AdmissionLimits{MaxInFlight: 2, MaxWaiting: 4, MaxQueueWait: time.Second},
		Namespace:         "runtime-test", SecretsFile: "/never-read-in-invalid-config/synthetic-secret", LoginWindow: time.Minute, StartupTimeout: 3 * time.Second,
		HTTP: httptransport.Config{Origin: "https://oj.invalid", Timeout: time.Second}, Mode: DirectTLS,
		LoginLimits: redisstore.LoginLimits{BeginGlobal: b, BeginSource: b, BeginEmail: b, RecoveryGlobal: b, RecoverySource: b},
	}
}

func TestConfigRejectedBeforeSecretOrNetworkAccess(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Config)
	}{
		{"zero mode", func(c *Config) { c.Mode = 0 }}, {"unknown mode", func(c *Config) { c.Mode = 9 }},
		{"direct pins", func(c *Config) { c.Proxy.CertificateSHA256 = [][32]byte{{1}} }},
		{"proxy missing pins", func(c *Config) { c.Mode = ProxyMTLS }},
		{"namespace", func(c *Config) { c.Namespace = "INVALID" }}, {"origin", func(c *Config) { c.HTTP.Origin = "http://oj.invalid" }},
		{"window", func(c *Config) { c.LoginWindow = 0 }}, {"limiter", func(c *Config) { c.LoginLimits.BeginEmail.Count = 0 }},
		{"HTTP timeout", func(c *Config) { c.HTTP.Timeout = 0 }}, {"startup timeout", func(c *Config) { c.StartupTimeout = 0 }},
		{"password policy", func(c *Config) { c.PasswordPolicy.Current.MemoryKiB = 0 }},
		{"password envelope", func(c *Config) { c.PasswordLimits.MaxSaltBytes = 128; c.PasswordPolicy.Current.SaltBytes = 65 }},
		{"queue allocation", func(c *Config) { c.PasswordAdmission.MaxWaiting = 65537 }},
		{"DB host", func(c *Config) { c.Database.Host = "" }}, {"DB port", func(c *Config) { c.Database.Port = 0 }},
		{"DB security", func(c *Config) { c.Database.Security = 0 }}, {"DB connections", func(c *Config) { c.Database.Connections = 0 }},
		{"DB timeout", func(c *Config) { c.Database.Timeout = 0 }}, {"DB idle", func(c *Config) { c.Database.MaxIdle = 2 * time.Hour }},
		{"DB health", func(c *Config) { c.Database.HealthCheck = 0 }}, {"DB label", func(c *Config) { c.Database.ApplicationName = "" }},
		{"Redis security", func(c *Config) { c.RedisSecurity = 0 }}, {"Redis connections", func(c *Config) { c.Redis.Connections = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := validConfig()
			tc.change(&c)
			if r, err := Open(t.Context(), c); r != nil || !errors.Is(err, ErrConfig) {
				t.Fatal("invalid pure configuration reached file/network startup")
			}
		})
	}
}

func TestDatabaseUsesTypedEndpointAndRejectsAmbientSettings(t *testing.T) {
	t.Setenv("PGDATA", "/unused-image-metadata")
	t.Setenv("PG_MAJOR", "14")
	t.Setenv("PG_VERSION", "synthetic-image-version")
	c := validConfig().Database
	c.Host, c.Username, c.Database, c.Password = "/tmp/explicit-fixture", "typed-user", "typed-db", "quote' service=bad sslmode=allow"
	cfg, err := databaseConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	conn := cfg.ConnConfig
	if conn.Host != c.Host || conn.Port != c.Port || conn.User != c.Username || conn.Database != c.Database || conn.Password != c.Password || conn.TLSConfig != nil || len(conn.Fallbacks) != 0 || cfg.MaxConns != 2 || cfg.MinConns != 0 || cfg.MinIdleConns != 0 || cfg.PingTimeout != time.Second {
		t.Fatal("typed endpoint/budgets were not preserved")
	}
	if conn.RuntimeParams["statement_timeout"] != "1000" || conn.RuntimeParams["search_path"] != "public" || len(conn.RuntimeParams) != 5 {
		t.Fatal("unexpected runtime SQL settings")
	}
	secure := c
	secure.Host, secure.Security = "db.invalid", VerifiedTLS
	secure.TLS = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: "db.invalid", RootCAs: x509.NewCertPool()}
	verified, err := databaseConfig(secure)
	if err != nil {
		t.Fatal(err)
	}
	if verified.ConnConfig.TLSConfig == nil || verified.ConnConfig.TLSConfig.ServerName != "db.invalid" || verified.ConnConfig.TLSConfig.MinVersion != tls.VersionTLS12 ||
		verified.ConnConfig.TLSConfig.RootCAs == secure.TLS.RootCAs || len(verified.ConnConfig.Fallbacks) != 0 {
		t.Fatal("database config dropped TLS identity or retained fallback")
	}
	for _, name := range []string{"PGHOST", "PGPASSWORD", "PGSERVICE", "PGSERVICEFILE", "PGOPTIONS", "PGSSLMODE"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "synthetic-secret")
			if _, err := databaseConfig(c); !errors.Is(err, ErrConfig) || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("ambient settings not rejected safely")
			}
			if os.Getenv(name) != "synthetic-secret" {
				t.Fatal("runtime mutated process environment")
			}
		})
	}
}

func TestTLSChoiceAndConfigRedaction(t *testing.T) {
	good := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: "db.invalid", RootCAs: x509.NewCertPool()}
	copy, err := cloneTLS(VerifiedTLS, good)
	if err != nil || copy == good || copy.RootCAs == good.RootCAs {
		t.Fatal("TLS trust config not cloned")
	}
	for _, tc := range []struct {
		mode Security
		c    *tls.Config
	}{{0, nil}, {PrivateTransport, good}, {VerifiedTLS, nil}, {VerifiedTLS, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12, ServerName: "db.invalid"}}, {VerifiedTLS, &tls.Config{MinVersion: tls.VersionTLS12}}} {
		if _, err := cloneTLS(tc.mode, tc.c); !errors.Is(err, ErrConfig) {
			t.Fatal("unsafe/implicit storage transport accepted")
		}
	}
	c := validConfig()
	for _, value := range []any{c, &c, c.Database, &c.Database} {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%f", "%x", "%X", "%b", "%o", "%e", "%c", "%U", "%t", "%020.3v"} {
			if fmt.Sprintf(format, value) != value.(fmt.Stringer).String() {
				t.Fatal("configuration formatting did not remain fully redacted")
			}
		}
		body, err := json.Marshal(value)
		if err != nil || strings.Contains(string(body), "synthetic-secret") {
			t.Fatal("configuration JSON leaked credentials")
		}
	}
}

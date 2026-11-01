package runtime

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var applicationPattern = regexp.MustCompile(`^[a-z0-9_-]{1,63}$`)

func databaseConfig(c DatabaseConfig) (*pgxpool.Config, error) {
	if c.Host == "" || strings.ContainsAny(c.Host, ",\x00") || c.Port == 0 || c.Database == "" || c.Username == "" ||
		strings.ContainsAny(c.Database+c.Username+c.Password, "\x00") || !applicationPattern.MatchString(c.ApplicationName) ||
		c.Connections < 1 || c.Connections > 1024 || c.Timeout < time.Millisecond || c.Timeout > 5*time.Second || c.Timeout%time.Millisecond != 0 ||
		c.MaxLifetime <= 0 || c.MaxIdle <= 0 || c.MaxIdle > c.MaxLifetime || c.HealthCheck <= 0 || c.HealthCheck > c.MaxIdle {
		return nil, ErrConfig
	}
	secure, err := cloneTLS(c.Security, c.TLS)
	if err != nil || (filepath.IsAbs(c.Host) && secure != nil) {
		return nil, ErrConfig
	}
	// pgx intentionally merges libpq environment and implicit credential files.
	// Refuse ambient PG settings, then parse only an inert constant template.
	// Never clear process environment, interpolate credentials or parse a user DSN.
	// This list matches pgx v5.11.0 pgconn.parseEnvSettings. Image metadata such
	// as PG_MAJOR/PG_VERSION/PGDATA is not connection input. Recheck on upgrades.
	for _, name := range []string{"PGHOST", "PGPORT", "PGDATABASE", "PGUSER", "PGPASSWORD", "PGPASSFILE", "PGAPPNAME", "PGCONNECT_TIMEOUT",
		"PGSSLMODE", "PGSSLKEY", "PGSSLCERT", "PGSSLSNI", "PGSSLROOTCERT", "PGSSLPASSWORD", "PGSSLNEGOTIATION", "PGTARGETSESSIONATTRS",
		"PGSERVICE", "PGSERVICEFILE", "PGTZ", "PGOPTIONS", "PGMINPROTOCOLVERSION", "PGMAXPROTOCOLVERSION", "PGCHANNELBINDING", "PGREQUIREAUTH"} {
		if os.Getenv(name) != "" {
			return nil, ErrConfig
		}
	}
	cfg, err := pgxpool.ParseConfig("host=127.0.0.1 port=5432 user=oj_template dbname=oj_template password='' passfile=/dev/null sslmode=disable sslrootcert='' sslcert='' sslkey=''")
	if err != nil {
		return nil, ErrConfig
	}
	conn := cfg.ConnConfig
	conn.Host, conn.Port, conn.Database, conn.User, conn.Password = c.Host, c.Port, c.Database, c.Username, c.Password
	conn.TLSConfig, conn.Fallbacks, conn.ConnectTimeout = secure, nil, c.Timeout
	ms := strconv.FormatInt(c.Timeout.Milliseconds(), 10)
	conn.RuntimeParams = map[string]string{"search_path": "public", "application_name": c.ApplicationName,
		"statement_timeout": ms, "lock_timeout": ms, "idle_in_transaction_session_timeout": ms}
	cfg.MaxConns, cfg.MinConns, cfg.MinIdleConns = c.Connections, 0, 0
	cfg.MaxConnLifetime, cfg.MaxConnLifetimeJitter = c.MaxLifetime, 0
	cfg.MaxConnIdleTime, cfg.HealthCheckPeriod, cfg.PingTimeout = c.MaxIdle, c.HealthCheck, c.Timeout
	return cfg, nil
}

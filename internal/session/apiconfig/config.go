// Package apiconfig parses the bounded, nonsecret configuration for the opt-in
// session API. It performs no file, environment, certificate or network access.
package apiconfig

import (
	"errors"
	"time"

	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

// ErrConfig deliberately contains no decoder cause, input value or file path.
var ErrConfig = errors.New("invalid session API configuration")

// Config separates nonsecret transport material from the native runtime config.
// The caller resolves TLS and fixed environment credentials before Runtime.Open.
type Config struct {
	Runtime                    sessionruntime.Config
	DatabaseAuth, RedisAuth    string
	TLS, DatabaseTLS, RedisTLS Material
	Server                     ServerConfig
	PasswordBlocklistFile      string
	MinimumPasswordCharacters  int
	EmailBlocklistFile         string
	EmailMinimumCharacters     int
	EmailBudgetAuth            string
	EmailBudgetTLS             Material
}

// Material names explicit TLS files; Parse validates only their lexical paths.
type Material struct {
	CertificateFile string `json:"certificate_file"`
	KeyFile         string `json:"key_file"`
	CAFile          string `json:"ca_file"`
	ServerName      string `json:"server_name"`
}

// ServerConfig contains explicit bounded listener and shutdown budgets.
type ServerConfig struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxHeaderBytes    int
}

// Package runtime assembles one ordinary-session authority and owns its clients.
// It is an HTTPS-only Handler, not a listener, migration or recovery supervisor.
package runtime

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

var (
	ErrConfig  = errors.New("invalid session runtime configuration")
	ErrStartup = errors.New("session runtime startup failed")
	ErrSecrets = errors.New("session runtime secrets unavailable")
	ErrClose   = errors.New("session runtime resource close failed")
)

// Mode explicitly selects the existing direct TLS or authenticated proxy adapter.
type Mode uint8

const (
	DirectTLS Mode = iota + 1
	ProxyMTLS
)

// Security requires an explicit trusted private transport or verified TLS choice.
// PrivateTransport does not discover or prove that a network is actually private.
type Security uint8

const (
	PrivateTransport Security = iota + 1
	VerifiedTLS
)

// DatabaseConfig contains only one explicit endpoint and bounded pool settings.
// No DSN, PG environment, alternate hosts or callbacks are accepted. Implicit
// credentials/certificates are not loaded; pgx may stat its default locations.
type DatabaseConfig struct {
	Host, Database, Username, Password, ApplicationName string
	Port                                                uint16
	Security                                            Security
	TLS                                                 *tls.Config
	Connections                                         int32
	Timeout, MaxLifetime, MaxIdle, HealthCheck          time.Duration
}

func (DatabaseConfig) String() string     { return "[redacted session database configuration]" }
func (c DatabaseConfig) GoString() string { return c.String() }

// Format redacts supported value verbs, including numeric verbs which bypass
// Stringer. Invalid format diagnostics and reflection are outside this contract.
func (c DatabaseConfig) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(c.String())) }
func (c DatabaseConfig) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted session database configuration]"`), nil
}

// Config is a trusted typed configuration, not an HTTP/file decoder. Every
// budget/transport choice is explicit; ordinary session lifetimes remain fixed.
// Callers must not mutate config/TLS backing material during Open or later use.
// TLS listener/certificate setup and stopping that listener remain caller-owned.
type Config struct {
	Database                    DatabaseConfig
	Redis                       redisstore.ClientConfig
	RedisSecurity               Security
	PasswordPolicy              password.Policy
	PasswordLimits              password.Limits
	PasswordAdmission           password.AdmissionLimits
	Namespace, SecretsFile      string
	LoginWindow, StartupTimeout time.Duration
	LoginLimits                 redisstore.LoginLimits
	HTTP                        httptransport.Config
	Mode                        Mode
	Proxy                       httptransport.ProxyConfig
	PasswordChange              *PasswordChangeConfig
	PrivateDrafts               *PrivateDraftsConfig
	// TrainingHistory enables only the read-only personal history route.
	TrainingHistory bool
	// TrainingSubmissionDetail independently enables owner-only source and verdict reads.
	TrainingSubmissionDetail bool
	// LocalTrainingIntake opens browser submission only for the admitted local queue.
	LocalTrainingIntake bool
	// LocalJava21 enables Java submissions only with the admitted local intake.
	LocalJava21 bool
	// SessionDevices enables listing and revoking the caller's other sessions.
	SessionDevices bool
	// SessionRotation enables replacing the caller's personal credential.
	SessionRotation bool
	// ManagementSessions enables the separate platform administrator login.
	ManagementSessions bool
	// ManagementRoleChanges requires ManagementSessions and opens role writes.
	ManagementRoleChanges bool

	// EmailLimits optionally provisions the email marker during isolated
	// recovery. This alone does not enable any public account route.
	EmailLimits *redisstore.EmailLimits
	// EmailConfirmation explicitly enables both link confirmation routes.
	EmailConfirmation *password.Acceptance
	// EmailIntake explicitly enables registration and password reset requests.
	EmailIntake *EmailIntakeConfig
}

// PasswordChangeConfig explicitly enables authenticated changes. Nil preserves
// the six-route session-only contract; enabled limits/rules receive no defaults.
type PasswordChangeConfig struct {
	Limits     redisstore.PasswordChangeLimits
	Acceptance *password.Acceptance
}

func (Config) String() string     { return "[redacted session runtime configuration]" }
func (c Config) GoString() string { return c.String() }

// Format redacts supported value verbs, including numeric verbs which bypass
// Stringer. Invalid format diagnostics and reflection are outside this contract.
func (c Config) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(c.String())) }
func (c Config) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted session runtime configuration]"`), nil
}

func cloneTLS(mode Security, config *tls.Config) (*tls.Config, error) {
	switch mode {
	case PrivateTransport:
		if config != nil {
			return nil, ErrConfig
		}
		return nil, nil
	case VerifiedTLS:
		if config == nil || config.InsecureSkipVerify || config.ServerName == "" || config.MinVersion < tls.VersionTLS12 {
			return nil, ErrConfig
		}
		copy := config.Clone()
		if copy.RootCAs != nil {
			copy.RootCAs = copy.RootCAs.Clone()
		}
		return copy, nil
	default:
		return nil, ErrConfig
	}
}

func (c Config) validate() (*password.Hasher, *password.BoundedHasher, error) {
	if c.LocalJava21 && !c.LocalTrainingIntake {
		return nil, nil, ErrConfig
	}
	if c.LocalTrainingIntake && (!c.TrainingHistory || !c.TrainingSubmissionDetail ||
		c.PrivateDrafts == nil || !c.PrivateDrafts.PublicCatalog || !c.PrivateDrafts.hasLocalExecution()) {
		return nil, nil, ErrConfig
	}
	if c.ManagementRoleChanges && !c.ManagementSessions {
		return nil, nil, ErrConfig
	}
	if c.EmailLimits != nil && c.EmailLimits.Validate() != nil {
		return nil, nil, ErrConfig
	}
	if rules := c.EmailConfirmation; rules != nil && (c.EmailLimits == nil ||
		rules.MinimumCharacters() < password.MinimumNewPasswordCharacters || rules.MaxBytes() != c.PasswordLimits.MaxPasswordBytes) {
		return nil, nil, ErrConfig
	}
	if c.PrivateDrafts != nil && c.PrivateDrafts.Validate() != nil {
		return nil, nil, ErrConfig
	}
	if c.PrivateDrafts != nil && c.PrivateDrafts.hasLocalExecution() {
		origin, err := url.Parse(c.HTTP.Origin)
		if err != nil || !net.ParseIP(origin.Hostname()).IsLoopback() ||
			(c.PrivateDrafts.LocalExecution != nil && !c.PrivateDrafts.LocalExecution.Valid()) || c.Database.Host == "" || c.Database.Host[0] != '/' {
			return nil, nil, ErrConfig
		}
	}
	if c.StartupTimeout <= 0 || c.StartupTimeout > time.Minute || c.SecretsFile == "" || c.HTTP.Validate() != nil ||
		redisstore.ValidateSetup(c.Namespace, redisstore.OrdinaryPolicy(), c.HTTP.Origin, c.LoginWindow, c.LoginLimits) != nil {
		return nil, nil, ErrConfig
	}
	if change := c.PasswordChange; change != nil && (change.Limits.Validate() != nil || change.Acceptance.MinimumCharacters() < redisstore.MinimumNewPasswordCharacters || change.Acceptance.MaxBytes() != c.PasswordLimits.MaxPasswordBytes) {
		return nil, nil, ErrConfig
	}
	if (c.Mode == DirectTLS && len(c.Proxy.CertificateSHA256) != 0) ||
		(c.Mode == ProxyMTLS && c.Proxy.Validate() != nil) || (c.Mode != DirectTLS && c.Mode != ProxyMTLS) {
		return nil, nil, ErrConfig
	}
	h, err := password.New(c.PasswordPolicy, c.PasswordLimits)
	if err != nil {
		return nil, nil, ErrConfig
	}
	salt, digest := h.RecordSizeBounds()
	if salt > identity.StoredSaltBytes || digest > identity.StoredDigestBytes {
		return nil, nil, ErrConfig
	}
	// Bound channel allocation before calling the shared admission constructor.
	// These are implementation ceilings, not approved production budgets.
	if c.PasswordAdmission.MaxInFlight > 1024 || c.PasswordAdmission.MaxWaiting > 65536 {
		return nil, nil, ErrConfig
	}
	work, err := password.NewBounded(h, c.PasswordAdmission)
	if err != nil {
		return nil, nil, ErrConfig
	}
	return h, work, nil
}

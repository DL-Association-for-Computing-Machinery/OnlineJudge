package apiconfig

import (
	"encoding/hex"
	"strings"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

func (w document) config() (Config, error) {
	if w.Version != 1 || w.Database == nil || w.Redis == nil || w.Password == nil ||
		w.LoginLimits == nil || w.TLS == nil || w.Server == nil || w.ProxyFingerprints == nil {
		return Config{}, ErrConfig
	}
	c := Config{TLS: *w.TLS}
	r := &c.Runtime
	r.Namespace, r.SecretsFile = w.Namespace, w.SecretsFile
	r.TrainingHistory = w.TrainingHistory
	r.TrainingSubmissionDetail = w.TrainingSubmissionDetail
	r.LocalTrainingIntake = w.LocalTrainingIntake
	r.LocalJava21 = w.LocalJava21
	r.LocalPython3 = w.LocalPython3
	if (w.LocalJava21 || w.LocalPython3) && !w.LocalTrainingIntake && w.PublishedTraining == nil {
		return Config{}, ErrConfig
	}
	if p := w.PublishedTraining; p != nil {
		if p.Validate() != nil || w.PrivateDrafts != nil || w.LocalTrainingIntake || !w.TrainingHistory || !w.TrainingSubmissionDetail {
			return Config{}, ErrConfig
		}
		r.PublishedTraining = p
	}
	r.SessionDevices = w.SessionDevices
	r.SessionRotation = w.SessionRotation
	r.ManagementSessions = w.ManagementSessions
	r.ManagementRoleChanges = w.ManagementRoleChanges
	if r.ManagementRoleChanges && !r.ManagementSessions {
		return Config{}, ErrConfig
	}
	r.HTTP.Origin = w.Origin
	if !durations([]durationField{{w.LoginWindow, &r.LoginWindow}, {w.StartupTimeout, &r.StartupTimeout},
		{w.HTTPTimeout, &r.HTTP.Timeout}}) || r.StartupTimeout <= 0 || r.StartupTimeout > time.Minute ||
		r.HTTP.Validate() != nil || !validPath(r.SecretsFile, false) {
		return Config{}, ErrConfig
	}
	switch w.Mode {
	case "direct":
		r.Mode = sessionruntime.DirectTLS
		if len(w.ProxyFingerprints) != 0 {
			return Config{}, ErrConfig
		}
	case "proxy":
		r.Mode = sessionruntime.ProxyMTLS
		for _, encoded := range w.ProxyFingerprints {
			if len(encoded) != 64 || strings.ToLower(encoded) != encoded {
				return Config{}, ErrConfig
			}
			var pin [32]byte
			if _, err := hex.Decode(pin[:], []byte(encoded)); err != nil {
				return Config{}, ErrConfig
			}
			r.Proxy.CertificateSHA256 = append(r.Proxy.CertificateSHA256, pin)
		}
		if r.Proxy.Validate() != nil {
			return Config{}, ErrConfig
		}
	default:
		return Config{}, ErrConfig
	}
	if !listenerMaterial(c.TLS, r.Mode) || w.Database.apply(&r.Database, &c.DatabaseAuth, &c.DatabaseTLS) != nil || w.Redis.apply(&c) != nil ||
		w.Password.apply(r) != nil || w.LoginLimits.apply(r) != nil || w.Server.apply(&c) != nil {
		return Config{}, ErrConfig
	}
	if w.PasswordChange != nil && w.PasswordChange.apply(&c) != nil {
		return Config{}, ErrConfig
	}
	if w.EmailLimits != nil && w.EmailLimits.apply(r) != nil {
		return Config{}, ErrConfig
	}
	if confirm := w.EmailConfirmation; confirm != nil {
		if r.EmailLimits == nil || !validNewPasswordConfig(confirm.MinimumCharacters, confirm.BlocklistFile, r.PasswordLimits.MaxPasswordBytes) {
			return Config{}, ErrConfig
		}
		c.EmailMinimumCharacters, c.EmailBlocklistFile = confirm.MinimumCharacters, confirm.BlocklistFile
	}
	if w.EmailIntake != nil && w.EmailIntake.apply(&c) != nil {
		return Config{}, ErrConfig
	}
	if w.PrivateDrafts != nil && w.PrivateDrafts.apply(&c) != nil {
		return Config{}, ErrConfig
	}
	return c, nil
}

func (w emailLimitsWire) apply(r *sessionruntime.Config) error {
	limits := &redisstore.EmailLimits{}
	items := []struct {
		wire *bucketWire
		out  *redisstore.LoginBucket
	}{
		{w.RequestGlobal, &limits.RequestGlobal}, {w.RequestSource, &limits.RequestSource},
		{w.RequestMailbox, &limits.RequestMailbox}, {w.ConfirmGlobal, &limits.ConfirmGlobal},
		{w.ConfirmSource, &limits.ConfirmSource}, {w.ConfirmToken, &limits.ConfirmToken},
	}
	for _, item := range items {
		if item.wire == nil || !durations([]durationField{{item.wire.Window, &item.out.Window}}) {
			return ErrConfig
		}
		item.out.Count = item.wire.Count
	}
	if limits.Validate() != nil {
		return ErrConfig
	}
	r.EmailLimits = limits
	return nil
}

func (w privateDraftsWire) apply(c *Config) error {
	d := &sessionruntime.PrivateDraftsConfig{MaterialRoot: w.MaterialRoot, BundleDirectory: w.BundleDirectory,
		SourceLockFile: w.SourceLockFile, SourceIDs: append([]string(nil), w.SourceIDs...), MaxInFlight: w.MaxInFlight,
		ReferenceRequests: w.ReferenceRequests, ReviewIntake: w.ReviewIntake, ReviewAccess: w.ReviewAccess,
		ReviewReturns: w.ReviewReturns, ReviewApprovals: w.ReviewApprovals, PublicCatalog: w.PublicCatalog,
		StatementConfirmations: w.StatementConfirmations, LocalExecution: w.LocalExecution, LocalTextExecution: w.LocalTextExecution}
	if !durations([]durationField{{w.Timeout, &d.Timeout}}) || d.Validate() != nil || c.Server.WriteTimeout < d.Timeout {
		return ErrConfig
	}
	c.Runtime.PrivateDrafts = d
	return nil
}

func (w passwordChangeWire) apply(c *Config) error {
	if !validNewPasswordConfig(w.MinimumCharacters, w.BlocklistFile, c.Runtime.PasswordLimits.MaxPasswordBytes) {
		return ErrConfig
	}
	change := &sessionruntime.PasswordChangeConfig{}
	items := []struct {
		wire *bucketWire
		out  *redisstore.LoginBucket
	}{{w.Global, &change.Limits.Global}, {w.Source, &change.Limits.Source}, {w.Subject, &change.Limits.Subject}}
	for _, item := range items {
		if item.wire == nil || !durations([]durationField{{item.wire.Window, &item.out.Window}}) {
			return ErrConfig
		}
		item.out.Count = item.wire.Count
	}
	if change.Limits.Validate() != nil {
		return ErrConfig
	}
	c.Runtime.PasswordChange = change
	c.PasswordBlocklistFile, c.MinimumPasswordCharacters = w.BlocklistFile, w.MinimumCharacters
	return nil
}

func validNewPasswordConfig(minimum int, file string, maxBytes uint32) bool {
	return minimum >= password.MinimumNewPasswordCharacters && minimum <= 64 &&
		uint64(maxBytes) >= 4*uint64(minimum) && file != "" && validPath(file, false)
}

type durationField struct {
	text string
	out  *time.Duration
}

func durations(fields []durationField) bool {
	for _, field := range fields {
		value, err := time.ParseDuration(field.text)
		if field.text == "" || err != nil {
			return false
		}
		*field.out = value
	}
	return true
}

func (w databaseWire) apply(db *sessionruntime.DatabaseConfig, auth *string, tls *Material) error {
	if w.TLS == nil || !authMode(w.Auth) {
		return ErrConfig
	}
	db.Host, db.Port, db.Database, db.Username = w.Host, w.Port, w.Database, w.Username
	db.ApplicationName, db.Connections = w.ApplicationName, w.Connections
	security, ok := storageSecurity(w.Security, *w.TLS)
	if !ok || !durations([]durationField{{w.Timeout, &db.Timeout}, {w.MaxLifetime, &db.MaxLifetime},
		{w.MaxIdle, &db.MaxIdle}, {w.HealthCheck, &db.HealthCheck}}) {
		return ErrConfig
	}
	db.Security = security
	// Native Open owns complete endpoint/pool policy and ambient-PG rejection.
	// These cheap checks ensure required wire budgets never acquire defaults.
	if db.Host == "" || db.Port == 0 || db.Database == "" || db.Username == "" || db.ApplicationName == "" ||
		db.Connections <= 0 || db.Connections > 1024 || db.Timeout <= 0 || db.Timeout > 5*time.Second ||
		db.MaxLifetime <= 0 || db.MaxIdle <= 0 || db.HealthCheck <= 0 {
		return ErrConfig
	}
	*auth, *tls = w.Auth, *w.TLS
	return nil
}

func (w redisWire) apply(c *Config) error {
	if w.TLS == nil || !authMode(w.Auth) || (w.Auth == "none" && w.Username != "") {
		return ErrConfig
	}
	r := &c.Runtime.Redis
	r.Addr, r.Username, r.Connections = w.Address, w.Username, w.Connections
	security, ok := storageSecurity(w.Security, *w.TLS)
	if !ok || !durations([]durationField{{w.Timeout, &r.Timeout}}) || r.Addr == "" ||
		r.Connections <= 0 || r.Connections > 1024 || r.Timeout <= 0 || r.Timeout > 5*time.Second {
		return ErrConfig
	}
	c.Runtime.RedisSecurity = security
	c.RedisAuth, c.RedisTLS = w.Auth, *w.TLS
	return nil
}

func authMode(value string) bool { return value == "none" || value == "env" }

func parameters(w parametersWire) password.Parameters {
	return password.Parameters{MemoryKiB: w.MemoryKiB, Iterations: w.Iterations,
		Parallelism: w.Parallelism, SaltBytes: w.SaltBytes, DigestBytes: w.DigestBytes}
}

func (w passwordWire) apply(r *sessionruntime.Config) error {
	if w.Current == nil || w.Legacy == nil || w.Limits == nil || w.Admission == nil || w.Admission.MaxWaiting == nil {
		return ErrConfig
	}
	r.PasswordPolicy.Current = parameters(*w.Current)
	for _, item := range w.Legacy {
		r.PasswordPolicy.Legacy = append(r.PasswordPolicy.Legacy, parameters(item))
	}
	l := w.Limits
	r.PasswordLimits = password.Limits{MaxMemoryKiB: l.MaxMemoryKiB, MaxIterations: l.MaxIterations,
		MaxParallelism: l.MaxParallelism, MaxPasswordBytes: l.MaxPasswordBytes,
		MaxSaltBytes: l.MaxSaltBytes, MaxDigestBytes: l.MaxDigestBytes}
	hasher, err := password.New(r.PasswordPolicy, r.PasswordLimits)
	if err != nil {
		return ErrConfig
	}
	salt, digest := hasher.RecordSizeBounds()
	if salt > identity.StoredSaltBytes || digest > identity.StoredDigestBytes {
		return ErrConfig
	}
	a := w.Admission
	r.PasswordAdmission = password.AdmissionLimits{MaxInFlight: a.MaxInFlight, MaxWaiting: *a.MaxWaiting}
	if !durations([]durationField{{a.MaxQueueWait, &r.PasswordAdmission.MaxQueueWait}}) ||
		a.MaxInFlight > 1024 || *a.MaxWaiting > 65536 {
		return ErrConfig
	}
	if _, err := password.NewBounded(hasher, r.PasswordAdmission); err != nil {
		return ErrConfig
	}
	return nil
}

func (w limitsWire) apply(r *sessionruntime.Config) error {
	items := []struct {
		wire *bucketWire
		out  *redisstore.LoginBucket
	}{{w.BeginGlobal, &r.LoginLimits.BeginGlobal}, {w.BeginSource, &r.LoginLimits.BeginSource},
		{w.BeginEmail, &r.LoginLimits.BeginEmail}, {w.RecoveryGlobal, &r.LoginLimits.RecoveryGlobal},
		{w.RecoverySource, &r.LoginLimits.RecoverySource}}
	for _, item := range items {
		if item.wire == nil || !durations([]durationField{{item.wire.Window, &item.out.Window}}) {
			return ErrConfig
		}
		item.out.Count = item.wire.Count
	}
	if redisstore.ValidateSetup(r.Namespace, redisstore.OrdinaryPolicy(), r.HTTP.Origin, r.LoginWindow, r.LoginLimits) != nil {
		return ErrConfig
	}
	return nil
}

func (w serverWire) apply(c *Config) error {
	s := &c.Server
	s.MaxHeaderBytes = w.MaxHeaderBytes
	if !durations([]durationField{{w.ReadHeaderTimeout, &s.ReadHeaderTimeout}, {w.ReadTimeout, &s.ReadTimeout},
		{w.WriteTimeout, &s.WriteTimeout}, {w.IdleTimeout, &s.IdleTimeout}, {w.ShutdownTimeout, &s.ShutdownTimeout}}) {
		return ErrConfig
	}
	for _, budget := range []time.Duration{s.ReadHeaderTimeout, s.ReadTimeout, s.WriteTimeout, s.ShutdownTimeout} {
		if budget <= 0 || budget > time.Minute {
			return ErrConfig
		}
	}
	if s.IdleTimeout <= 0 || s.IdleTimeout > 10*time.Minute || s.ReadTimeout < s.ReadHeaderTimeout ||
		s.WriteTimeout < c.Runtime.HTTP.Timeout || s.MaxHeaderBytes < 1024 || s.MaxHeaderBytes > 1024*1024 {
		return ErrConfig
	}
	return nil
}

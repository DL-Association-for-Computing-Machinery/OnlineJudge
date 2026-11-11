package apiconfig

type document struct {
	Version           int                 `json:"version"`
	Mode              string              `json:"mode"`
	Namespace         string              `json:"namespace"`
	SecretsFile       string              `json:"secrets_file"`
	Origin            string              `json:"origin"`
	LoginWindow       string              `json:"login_window"`
	StartupTimeout    string              `json:"startup_timeout"`
	HTTPTimeout       string              `json:"http_timeout"`
	Database          *databaseWire       `json:"database"`
	Redis             *redisWire          `json:"redis"`
	Password          *passwordWire       `json:"password"`
	LoginLimits       *limitsWire         `json:"login_limits"`
	ProxyFingerprints []string            `json:"proxy_fingerprints"`
	TLS               *Material           `json:"tls"`
	Server            *serverWire         `json:"server"`
	PasswordChange    *passwordChangeWire `json:"password_change"`
	PrivateDrafts     *privateDraftsWire  `json:"private_drafts"`
}

type privateDraftsWire struct {
	MaterialRoot      string   `json:"material_root"`
	BundleDirectory   string   `json:"bundle_directory"`
	SourceLockFile    string   `json:"source_lock_file"`
	SourceIDs         []string `json:"source_ids"`
	Timeout           string   `json:"timeout"`
	MaxInFlight       int      `json:"max_in_flight"`
	ReferenceRequests bool     `json:"reference_requests"`
	ReviewIntake      bool     `json:"review_intake"`
	ReviewAccess      bool     `json:"review_access"`
	ReviewReturns     bool     `json:"review_returns"`
}

type passwordChangeWire struct {
	MinimumCharacters int         `json:"minimum_characters"`
	BlocklistFile     string      `json:"blocklist_file"`
	Global            *bucketWire `json:"global"`
	Source            *bucketWire `json:"source"`
	Subject           *bucketWire `json:"subject"`
}

type databaseWire struct {
	Host            string    `json:"host"`
	Port            uint16    `json:"port"`
	Database        string    `json:"database"`
	Username        string    `json:"username"`
	ApplicationName string    `json:"application_name"`
	Security        string    `json:"security"`
	Auth            string    `json:"auth"`
	Connections     int32     `json:"connections"`
	Timeout         string    `json:"timeout"`
	MaxLifetime     string    `json:"max_lifetime"`
	MaxIdle         string    `json:"max_idle"`
	HealthCheck     string    `json:"health_check"`
	TLS             *Material `json:"tls"`
}

type redisWire struct {
	Address     string    `json:"address"`
	Username    string    `json:"username"`
	Security    string    `json:"security"`
	Auth        string    `json:"auth"`
	Connections int       `json:"connections"`
	Timeout     string    `json:"timeout"`
	TLS         *Material `json:"tls"`
}

type parametersWire struct {
	MemoryKiB   uint32 `json:"memory_kib"`
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
	SaltBytes   uint32 `json:"salt_bytes"`
	DigestBytes uint32 `json:"digest_bytes"`
}

type passwordLimitsWire struct {
	MaxMemoryKiB     uint32 `json:"max_memory_kib"`
	MaxIterations    uint32 `json:"max_iterations"`
	MaxParallelism   uint8  `json:"max_parallelism"`
	MaxPasswordBytes uint32 `json:"max_password_bytes"`
	MaxSaltBytes     uint32 `json:"max_salt_bytes"`
	MaxDigestBytes   uint32 `json:"max_digest_bytes"`
}

type admissionWire struct {
	MaxInFlight  int    `json:"max_in_flight"`
	MaxWaiting   *int   `json:"max_waiting"`
	MaxQueueWait string `json:"max_queue_wait"`
}

type passwordWire struct {
	Current   *parametersWire     `json:"current"`
	Legacy    []parametersWire    `json:"legacy"`
	Limits    *passwordLimitsWire `json:"limits"`
	Admission *admissionWire      `json:"admission"`
}

type bucketWire struct {
	Count  int    `json:"count"`
	Window string `json:"window"`
}

type limitsWire struct {
	BeginGlobal    *bucketWire `json:"begin_global"`
	BeginSource    *bucketWire `json:"begin_source"`
	BeginEmail     *bucketWire `json:"begin_email"`
	RecoveryGlobal *bucketWire `json:"recovery_global"`
	RecoverySource *bucketWire `json:"recovery_source"`
}

type serverWire struct {
	ReadHeaderTimeout string `json:"read_header_timeout"`
	ReadTimeout       string `json:"read_timeout"`
	WriteTimeout      string `json:"write_timeout"`
	IdleTimeout       string `json:"idle_timeout"`
	ShutdownTimeout   string `json:"shutdown_timeout"`
	MaxHeaderBytes    int    `json:"max_header_bytes"`
}

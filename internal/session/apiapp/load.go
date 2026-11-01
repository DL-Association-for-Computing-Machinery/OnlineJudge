// Package apiapp runs the explicitly enabled ordinary-session HTTPS service.
package apiapp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/apiconfig"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

var (
	ErrConfiguration = errors.New("session API configuration unavailable or invalid")
	ErrTLS           = errors.New("session API TLS material unavailable or invalid")
	ErrListen        = errors.New("session API listener unavailable")
	ErrShutdown      = errors.New("session API shutdown incomplete")
)

// Lookup supplies only the two fixed password environment names, never arbitrary
// names from JSON. An explicit none mode rejects leftover nonempty credentials.
type Lookup func(string) (string, bool)

func loadConfig(ctx context.Context, path string, lookup Lookup) (apiconfig.Config, error) {
	if lookup == nil {
		return apiconfig.Config{}, ErrConfiguration
	}
	body, err := checkedfile.ReadPublic(path, 64*1024)
	if err != nil {
		return apiconfig.Config{}, ErrConfiguration
	}
	defer clear(body)
	c, err := apiconfig.Parse(body)
	if err != nil {
		return apiconfig.Config{}, ErrConfiguration
	}
	if err := ctx.Err(); err != nil {
		return apiconfig.Config{}, err
	}
	c.Runtime.Database.Password, err = password(c.DatabaseAuth, "OJ_SESSION_DATABASE_PASSWORD", lookup)
	if err != nil {
		return apiconfig.Config{}, err
	}
	c.Runtime.Redis.Password, err = password(c.RedisAuth, "OJ_SESSION_REDIS_PASSWORD", lookup)
	if err != nil {
		return apiconfig.Config{}, err
	}
	return c, nil
}

func password(mode, name string, lookup Lookup) (string, error) {
	value, present := lookup(name)
	switch mode {
	case "none":
		if value != "" {
			return "", ErrConfiguration
		}
		return "", nil
	case "env":
		if !present || value == "" {
			return "", ErrConfiguration
		}
		return value, nil
	default:
		return "", ErrConfiguration
	}
}

func loadTLS(c *apiconfig.Config) (*tls.Config, error) {
	serverCert, err := keyPair(c.TLS, true)
	if err != nil {
		return nil, err
	}
	server := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{serverCert}}
	if c.Runtime.Mode == sessionruntime.ProxyMTLS {
		server.ClientCAs, err = roots(c.TLS.CAFile)
		if err != nil {
			return nil, err
		}
		server.ClientAuth = tls.RequireAndVerifyClientCert
	}
	c.Runtime.Database.TLS, err = storageTLS(c.Runtime.Database.Security, c.DatabaseTLS)
	if err != nil {
		return nil, err
	}
	c.Runtime.Redis.TLS, err = storageTLS(c.Runtime.RedisSecurity, c.RedisTLS)
	if err != nil {
		return nil, err
	}
	return server, nil
}

func roots(path string) (*x509.CertPool, error) {
	body, err := checkedfile.ReadPublic(path, 1<<20)
	if err != nil {
		return nil, ErrTLS
	}
	defer clear(body)
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(body) {
		return nil, ErrTLS
	}
	return pool, nil
}

func keyPair(m apiconfig.Material, server bool) (tls.Certificate, error) {
	cert, err := checkedfile.ReadPublic(m.CertificateFile, 64*1024)
	if err != nil {
		return tls.Certificate{}, ErrTLS
	}
	defer clear(cert)
	key, err := checkedfile.ReadSecret(m.KeyFile, 64*1024)
	if err != nil {
		return tls.Certificate{}, ErrTLS
	}
	defer clear(key)
	pair, err := tls.X509KeyPair(cert, key)
	if err != nil || len(pair.Certificate) == 0 {
		return tls.Certificate{}, ErrTLS
	}
	pair.Leaf, err = x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return tls.Certificate{}, ErrTLS
	}
	now := time.Now()
	if now.Before(pair.Leaf.NotBefore) || !now.Before(pair.Leaf.NotAfter) {
		return tls.Certificate{}, ErrTLS
	}
	want := x509.ExtKeyUsageClientAuth
	if server {
		want = x509.ExtKeyUsageServerAuth
	}
	allowed := len(pair.Leaf.ExtKeyUsage) == 0
	for _, usage := range pair.Leaf.ExtKeyUsage {
		if usage == want || usage == x509.ExtKeyUsageAny {
			allowed = true
		}
	}
	if !allowed {
		return tls.Certificate{}, ErrTLS
	}
	return pair, nil
}

func storageTLS(security sessionruntime.Security, m apiconfig.Material) (*tls.Config, error) {
	if security == sessionruntime.PrivateTransport {
		return nil, nil
	}
	pool, err := roots(m.CAFile)
	if err != nil {
		return nil, err
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: m.ServerName, RootCAs: pool}
	if m.CertificateFile != "" {
		pair, err := keyPair(m, false)
		if err != nil {
			return nil, err
		}
		config.Certificates = []tls.Certificate{pair}
	}
	return config, nil
}

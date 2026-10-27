// Package testpki creates ephemeral certificates exclusively for protocol tests.
package testpki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Authority holds an ephemeral test CA; nothing is persisted outside t.TempDir.
type Authority struct {
	cert   *x509.Certificate
	key    *ecdsa.PrivateKey
	Roots  *x509.CertPool
	CAFile string
}

// New creates an independent, test-only trust root.
func New(t *testing.T) *Authority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(t, err)
	cert := &x509.Certificate{SerialNumber: serial(t), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	check(t, err)
	cert, err = x509.ParseCertificate(der)
	check(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	path := filepath.Join(t.TempDir(), "ca.pem")
	check(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600))
	return &Authority{cert: cert, key: key, Roots: roots, CAFile: path}
}

// Issue creates a client or localhost server certificate and private temporary files.
func (a *Authority) Issue(t *testing.T, identity string, usage x509.ExtKeyUsage, until time.Time) (tls.Certificate, string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(t, err)
	cert := &x509.Certificate{SerialNumber: serial(t), NotBefore: time.Now().Add(-2 * time.Hour), NotAfter: until,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	if identity != "" {
		u, err := url.Parse(identity)
		check(t, err)
		cert.URIs = []*url.URL{u}
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, a.cert, &key.PublicKey, a.key)
	check(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	check(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	check(t, os.WriteFile(certFile, certPEM, 0600))
	check(t, os.WriteFile(keyFile, keyPEM, 0600))
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	check(t, err)
	return pair, certFile, keyFile
}

func serial(t *testing.T) *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	check(t, err)
	return n.Add(n, big.NewInt(1))
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Package workerapi serves the private, authenticated controller protocol.
package workerapi

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

var identityURI = regexp.MustCompile(`^spiffe://onlinejudge/worker/([A-Za-z0-9_-]{1,128})/([1-9][0-9]{0,18})$`)

// LoadTLS requires explicit trust roots and an owner-private server key. It never
// falls back to system roots, plaintext, forwarded headers or optional client auth.
func LoadTLS(caFile, certFile, keyFile string) (*tls.Config, error) {
	info, err := os.Stat(keyFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("worker API key must be an owner-private regular file")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, errors.New("cannot load worker API certificate and key")
	}
	data, err := os.ReadFile(caFile)
	if err != nil {
		return nil, errors.New("cannot load worker client CA")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		return nil, errors.New("invalid worker client CA")
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs: roots, Certificates: []tls.Certificate{cert},
		// Resume must not preserve an earlier certificate validation decision.
		SessionTicketsDisabled: true,
	}, nil
}

func authenticate(state *tls.ConnectionState, roots *x509.CertPool, now time.Time) (submission.NodeIdentity, error) {
	if state == nil || len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 || roots == nil {
		return submission.NodeIdentity{}, submission.ErrNode
	}
	leaf := state.PeerCertificates[0]
	intermediates := x509.NewCertPool()
	for _, cert := range state.PeerCertificates[1:] {
		intermediates.AddCert(cert)
	}
	// Handshake verification alone would allow an expired certificate to keep
	// using an already established connection. Recheck the chain on every request.
	_, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates,
		CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	if err != nil || len(leaf.URIs) != 1 {
		return submission.NodeIdentity{}, submission.ErrNode
	}
	parts := identityURI.FindStringSubmatch(leaf.URIs[0].String())
	if len(parts) != 3 {
		return submission.NodeIdentity{}, submission.ErrNode
	}
	version, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return submission.NodeIdentity{}, submission.ErrNode
	}
	return submission.NodeIdentity{ID: parts[1], CredentialVersion: version}, nil
}

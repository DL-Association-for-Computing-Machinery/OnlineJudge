// Package workerclient implements bounded, authenticated node-side operations.
package workerclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// Renewer binds credentials to one HTTPS controller. It never follows redirects
// or includes the lease token, response body, or URL in returned errors.
type Renewer struct {
	endpoint string
	client   *http.Client
}

// NewRenewer requires explicit trust roots and a node certificate. Certificate
// loading is a deployment concern; this constructor neither reads files nor uses
// system roots. The caller owns these immutable credentials for this client.
func NewRenewer(endpoint string, roots *x509.CertPool, cert tls.Certificate) (*Renewer, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery ||
		roots == nil || len(cert.Certificate) == 0 || cert.PrivateKey == nil {
		return nil, errors.New("explicit HTTPS controller, roots and node credentials required")
	}
	u.Path = "/api/v1/worker/renew"
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13,
		RootCAs: roots.Clone(), Certificates: []tls.Certificate{cert}},
		MaxConnsPerHost: 1, MaxIdleConnsPerHost: 1, IdleConnTimeout: 30 * time.Second,
		TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 3 * time.Second}
	return &Renewer{endpoint: u.String(), client: &http.Client{Transport: transport, Timeout: 3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}}, nil
}

var tokenPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Renew acknowledges only a bounded, valid success response. All unknown or
// failed outcomes stop the caller's attempt; there is no automatic retry.
func (r *Renewer) Renew(ctx context.Context, token string) error {
	if !tokenPattern.MatchString(token) {
		return submission.ErrLease
	}
	body, _ := json.Marshal(struct {
		Token string `json:"lease_token"`
	}{token})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("cannot construct renewal")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return errors.New("renewal transport unavailable")
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return submission.ErrNode
	case http.StatusConflict:
		return submission.ErrLease
	case http.StatusOK:
	default:
		return errors.New("renewal not acknowledged")
	}
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return errors.New("invalid renewal media type")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1025))
	if err != nil || len(data) > 1024 {
		return errors.New("renewal response unavailable or oversized")
	}
	var response struct {
		Until time.Time `json:"lease_until"`
	}
	if json.Unmarshal(data, &response) != nil || response.Until.IsZero() {
		return errors.New("invalid renewal acknowledgement")
	}
	return nil
}

// Close releases idle TLS connections when the owning worker shuts down.
func (r *Renewer) Close() { r.client.CloseIdleConnections() }

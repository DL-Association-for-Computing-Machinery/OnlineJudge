package httptransport

import (
	"crypto/sha256"
	"net/http"
	"net/netip"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ProxyConfig pins the leaf certificates of explicitly authorized TLS proxies.
// The backend server must verify client certificates; the proxy must overwrite
// X-Forwarded-For/Host/Proto from its actual client connection and serve HTTPS.
// This supports one trusted hop, not appended forwarding chains or plaintext.
type ProxyConfig struct {
	CertificateSHA256 [][32]byte
}

type proxyTrust struct {
	certificates map[[32]byte]struct{}
}

// NewProxiedLimitedLogin requires admission, verified and pinned proxy mTLS,
// and exactly one forwarded client IP, public host and HTTPS scheme. It never
// falls back to direct mode. Pins are copied; rotation requires a new handler.
// This authenticates the proxy's assertions, not its public TLS configuration.
func NewProxiedLimitedLogin(login *redisstore.Login, config Config, proxy ProxyConfig, change ...*redisstore.PasswordChange) (http.Handler, error) {
	if err := proxy.Validate(); err != nil {
		return nil, err
	}
	pins := make(map[[32]byte]struct{}, len(proxy.CertificateSHA256))
	for _, pin := range proxy.CertificateSHA256 {
		pins[pin] = struct{}{}
	}
	base, err := NewLimitedLogin(login, config, change...)
	if err != nil {
		return nil, err
	}
	h := base.(*handler)
	h.proxy = &proxyTrust{certificates: pins}
	return h, nil
}

// Validate checks the explicit proxy fingerprint set without a manager or I/O.
func (proxy ProxyConfig) Validate() error {
	if len(proxy.CertificateSHA256) == 0 || len(proxy.CertificateSHA256) > 16 {
		return redisstore.ErrInvalid
	}
	pins := make(map[[32]byte]struct{}, len(proxy.CertificateSHA256))
	for _, pin := range proxy.CertificateSHA256 {
		if pin == ([32]byte{}) {
			return redisstore.ErrInvalid
		}
		if _, exists := pins[pin]; exists {
			return redisstore.ErrInvalid
		}
		pins[pin] = struct{}{}
	}
	return nil
}

func (h *handler) requestSource(r *http.Request) (netip.Addr, bool) {
	if r.TLS == nil || r.Host != h.host {
		return netip.Addr{}, false
	}
	if h.proxy == nil {
		return netip.Addr{}, true
	}
	if len(r.TLS.PeerCertificates) == 0 || r.TLS.PeerCertificates[0] == nil {
		return netip.Addr{}, false
	}
	leaf := r.TLS.PeerCertificates[0]
	if _, allowed := h.proxy.certificates[sha256.Sum256(leaf.Raw)]; !allowed {
		return netip.Addr{}, false
	}
	verified := false
	for _, chain := range r.TLS.VerifiedChains {
		if len(chain) > 0 && chain[0] != nil && leaf.Equal(chain[0]) {
			verified = true
			break
		}
	}
	if !verified || !singleHeader(r.Header, "X-Forwarded-Host", h.host) ||
		!singleHeader(r.Header, "X-Forwarded-Proto", "https") {
		return netip.Addr{}, false
	}
	// Other forwarding dialects are rejected rather than choosing precedence.
	if _, present := r.Header["Forwarded"]; present {
		return netip.Addr{}, false
	}
	if _, present := r.Header["X-Real-Ip"]; present {
		return netip.Addr{}, false
	}
	values := r.Header.Values("X-Forwarded-For")
	if len(values) != 1 {
		return netip.Addr{}, false
	}
	source, err := netip.ParseAddr(values[0])
	if err != nil || source.Zone() != "" {
		return netip.Addr{}, false
	}
	source = source.Unmap()
	if source.IsUnspecified() || source.IsMulticast() {
		return netip.Addr{}, false
	}
	return source, true
}

func singleHeader(header http.Header, name, value string) bool {
	values := header.Values(name)
	return len(values) == 1 && values[0] == value
}

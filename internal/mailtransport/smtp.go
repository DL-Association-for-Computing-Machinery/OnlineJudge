// Package mailtransport sends a single account email using verified SMTP TLS.
package mailtransport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"time"

	mail "github.com/wneessen/go-mail"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// Outcome describes the provider response without storing server text.
type Outcome uint8

const (
	// PermanentFailure is a definite refusal or invalid input/configuration.
	PermanentFailure Outcome = iota
	// Accepted means the server acknowledged DATA, not delivery to the inbox.
	Accepted
	// TemporaryFailure is safe to retry within the caller's budget.
	TemporaryFailure
	// Unknown means the final DATA reply was lost; retries may duplicate mail.
	Unknown
)

// TLSMode selects an encrypted SMTP transport with no plaintext fallback.
type TLSMode uint8

const (
	// ImplicitTLS starts TLS before SMTP.
	ImplicitTLS TLSMode = iota + 1
	// RequireSTARTTLS requires STARTTLS before authentication and sending.
	RequireSTARTTLS
)

// Config is trusted operator configuration. RootCAs is nil for system trust.
type Config struct {
	Host    string
	Port    int
	TLS     TLSMode
	From    identity.Email
	Timeout time.Duration
	RootCAs *x509.CertPool
}

// Sender is immutable and makes a new SMTP connection for each attempt.
type Sender struct {
	config   Config
	user     string
	password string
}

// Format excludes SMTP credentials from direct diagnostics.
func (Sender) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte("[redacted SMTP sender]")) }

// New validates configuration without making a network connection. Providing
// both credentials selects PLAIN authentication inside verified TLS.
func New(config Config, user, password string) (*Sender, error) {
	if config.Host == "" || len(config.Host) > 253 || strings.ContainsAny(config.Host, " /\\@?#[]\r\n\t%") ||
		config.Port < 1 || config.Port > 65535 || (config.TLS != ImplicitTLS && config.TLS != RequireSTARTTLS) ||
		!config.From.Valid() || config.Timeout <= 0 || config.Timeout > 30*time.Second ||
		(user == "") != (password == "") || len(user) > 512 || len(password) > 4096 ||
		strings.ContainsRune(user, 0) || strings.ContainsRune(password, 0) {
		return nil, errors.New("invalid SMTP configuration")
	}
	if strings.Contains(config.Host, ":") && net.ParseIP(config.Host) == nil {
		return nil, errors.New("invalid SMTP configuration")
	}
	if config.RootCAs != nil {
		config.RootCAs = config.RootCAs.Clone()
	}
	return &Sender{config: config, user: user, password: password}, nil
}

// Send performs one attempt. The caller owns eligibility, retries and queue
// state. Subject and text must come from a trusted account mail renderer.
func (s *Sender) Send(ctx context.Context, to identity.Email, subject, text string) Outcome {
	if s == nil || s.config.Host == "" || ctx == nil || !to.Valid() || subject == "" || len(subject) > 256 ||
		strings.ContainsAny(subject, "\r\n\x00") || text == "" || len(text) > 8192 {
		return PermanentFailure
	}
	if ctx.Err() != nil {
		return TemporaryFailure
	}
	ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()
	deadline, _ := ctx.Deadline()
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: s.config.Host, RootCAs: s.config.RootCAs}
	var stopClosing func() bool
	options := []mail.Option{
		mail.WithPort(s.config.Port), mail.WithTimeout(s.config.Timeout), mail.WithTLSConfig(cfg),
		mail.WithDialContextFunc(func(dialCtx context.Context, network, address string) (net.Conn, error) {
			raw, err := (&net.Dialer{}).DialContext(dialCtx, network, address)
			if err != nil {
				return nil, err
			}
			stopClosing = context.AfterFunc(ctx, func() { _ = raw.Close() })
			conn := boundedConn{Conn: raw, deadline: deadline}
			if s.config.TLS == ImplicitTLS {
				secure := tls.Client(conn, cfg)
				if err := secure.HandshakeContext(dialCtx); err != nil {
					_ = raw.Close()
					return nil, err
				}
				return secure, nil
			}
			return conn, nil
		}),
	}
	defer func() {
		if stopClosing != nil {
			stopClosing()
		}
	}()
	if s.config.TLS == ImplicitTLS {
		options = append(options, mail.WithSSL())
	} else {
		options = append(options, mail.WithTLSPolicy(mail.TLSMandatory))
	}
	if s.user != "" {
		options = append(options, mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithUsername(s.user), mail.WithPassword(s.password))
	}
	client, err := mail.NewClient(s.config.Host, options...)
	if err != nil {
		return PermanentFailure
	}
	msg := mail.NewMsg()
	if msg.From(s.config.From.Original()) != nil || msg.To(to.Original()) != nil {
		return PermanentFailure
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, text)
	conn, err := client.DialToSMTPClientWithContext(ctx)
	if err != nil {
		return dialOutcome(err)
	}
	defer func() { _ = conn.Close() }()
	if state, ok := conn.TLSConnectionState(); !ok || !state.HandshakeComplete || len(state.VerifiedChains) == 0 {
		return PermanentFailure
	}
	err = client.SendWithSMTPClient(conn, msg)
	if msg.IsDelivered() {
		return Accepted
	}
	var sendErr *mail.SendError
	if errors.As(err, &sendErr) {
		switch code := sendErr.ErrorCode(); {
		case code >= 500 && code < 600:
			return PermanentFailure
		case code >= 400 && code < 500:
			return TemporaryFailure
		case sendErr.Reason == mail.ErrSMTPDataClose:
			return Unknown
		}
	}
	return TemporaryFailure
}

func dialOutcome(err error) Outcome {
	var certificate *tls.CertificateVerificationError
	var reply *textproto.Error
	if errors.As(err, &certificate) || errors.Is(err, mail.ErrPlainAuthNotSupported) ||
		(errors.As(err, &reply) && reply.Code >= 500 && reply.Code < 600) {
		return PermanentFailure
	}
	return TemporaryFailure
}

// The library updates the socket timeout after connecting; keep the whole
// attempt within the caller's deadline, including greeting, AUTH and DATA.
type boundedConn struct {
	net.Conn
	deadline time.Time
}

func (c boundedConn) SetDeadline(t time.Time) error {
	if t.IsZero() || t.After(c.deadline) {
		t = c.deadline
	}
	return c.Conn.SetDeadline(t)
}

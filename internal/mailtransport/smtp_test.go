package mailtransport_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailtransport"
)

func TestLocalSMTP(t *testing.T) {
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cert := fixture.TLS.Certificates[0]
	roots := x509.NewCertPool()
	roots.AddCert(fixture.Certificate())
	fixture.Close()
	from, err := identity.ParseEmail("sender@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	to, err := identity.ParseEmail("recipient@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		mode mailtransport.TLSMode
		want mailtransport.Outcome
	}{
		{"accepted", mailtransport.ImplicitTLS, mailtransport.Accepted},
		{"accepted-disconnect", mailtransport.ImplicitTLS, mailtransport.Accepted},
		{"reject", mailtransport.ImplicitTLS, mailtransport.PermanentFailure},
		{"retry", mailtransport.ImplicitTLS, mailtransport.TemporaryFailure},
		{"lost-ack", mailtransport.ImplicitTLS, mailtransport.Unknown},
		{"data-timeout", mailtransport.ImplicitTLS, mailtransport.Unknown},
		{"data-retry", mailtransport.ImplicitTLS, mailtransport.TemporaryFailure},
		{"data-reject", mailtransport.ImplicitTLS, mailtransport.PermanentFailure},
		{"untrusted", mailtransport.ImplicitTLS, mailtransport.PermanentFailure},
		{"timeout", mailtransport.ImplicitTLS, mailtransport.TemporaryFailure},
		{"starttls", mailtransport.RequireSTARTTLS, mailtransport.Accepted},
		{"no-starttls", mailtransport.RequireSTARTTLS, mailtransport.TemporaryFailure},
		{"auth-rejected", mailtransport.ImplicitTLS, mailtransport.PermanentFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = listener.Close() }()
			if err := listener.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- receive(listener, cert, tc.name, tc.mode) }()
			trust := roots
			if tc.name == "untrusted" {
				trust = x509.NewCertPool()
			}
			user, password := "", ""
			if tc.name == "auth-rejected" || tc.name == "no-starttls" {
				user, password = "synthetic-fixture-user", "synthetic-fixture-password"
			}
			sender, err := mailtransport.New(mailtransport.Config{
				Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, TLS: tc.mode, From: from,
				Timeout: 3 * time.Second, RootCAs: trust,
			}, user, password)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprintf("%#v", sender) != "[redacted SMTP sender]" {
				t.Fatal("unexpected SMTP diagnostics")
			}
			ctx := t.Context()
			if tc.name == "timeout" || tc.name == "data-timeout" {
				short, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
				defer cancel()
				ctx = short
			}
			start := time.Now()
			got := sender.Send(ctx, to, "Local SMTP test", "synthetic fixture")
			if got != tc.want {
				t.Errorf("outcome = %d, want %d", got, tc.want)
			}
			if (tc.name == "timeout" || tc.name == "data-timeout") && time.Since(start) > 2*time.Second {
				t.Error("caller deadline did not bound SMTP attempt")
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func receive(listener *net.TCPListener, cert tls.Certificate, scenario string, mode mailtransport.TLSMode) error {
	raw, err := listener.AcceptTCP()
	if err != nil {
		return err
	}
	defer func() { _ = raw.Close() }()
	if err := raw.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}
	var conn net.Conn = raw
	upgrade := func() error {
		secure := tls.Server(raw, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
		conn = secure
		return secure.Handshake()
	}
	if mode == mailtransport.ImplicitTLS {
		err := upgrade()
		if scenario == "untrusted" {
			if err == nil {
				return errors.New("untrusted TLS handshake succeeded")
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
	if scenario == "timeout" {
		_, err := conn.Read(make([]byte, 1))
		if err != nil {
			return nil
		}
		return errors.New("SMTP command sent without greeting")
	}
	reply := func(s string) error { _, err := fmt.Fprintf(conn, "%s\r\n", s); return err }
	if err := reply("220 local.example.invalid ESMTP"); err != nil {
		return err
	}
	r := bufio.NewReader(conn)
	received := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && scenario == "no-starttls" {
				return nil
			}
			return err
		}
		switch {
		case strings.HasPrefix(line, "EHLO "):
			switch {
			case scenario == "auth-rejected", scenario == "no-starttls":
				err = reply("250-local\r\n250 AUTH PLAIN")
			case conn == raw && scenario == "starttls":
				err = reply("250-local\r\n250 STARTTLS")
			default:
				err = reply("250 OK")
			}
		case line == "STARTTLS\r\n" && scenario == "starttls":
			if err := reply("220 Ready"); err != nil {
				return err
			}
			if err := upgrade(); err != nil {
				return err
			}
			r = bufio.NewReader(conn)
		case strings.HasPrefix(line, "AUTH PLAIN ") && scenario == "auth-rejected":
			return reply("535 Authentication rejected")
		case conn == raw:
			return errors.New("SMTP sent credentials or envelope before TLS")
		case strings.HasPrefix(line, "NOOP"), strings.HasPrefix(line, "MAIL FROM:"):
			err = reply("250 OK")
		case strings.HasPrefix(line, "RSET"):
			err = reply("250 OK")
			if received {
				return err
			}
		case strings.HasPrefix(line, "RCPT TO:"):
			switch scenario {
			case "reject":
				return reply("550 No such mailbox")
			case "retry":
				return reply("450 Try later")
			}
			err = reply("250 OK")
		case line == "DATA\r\n":
			if err := reply("354 Continue"); err != nil {
				return err
			}
			for {
				line, err := r.ReadString('\n')
				if err != nil {
					return err
				}
				if line == ".\r\n" {
					break
				}
			}
			switch scenario {
			case "lost-ack":
				return nil
			case "data-retry":
				return reply("451 Try later")
			case "data-reject":
				return reply("554 Message rejected")
			case "data-timeout":
				_, err := r.ReadByte()
				if err == nil {
					return errors.New("unexpected command before final DATA response")
				}
				return nil
			}
			received = true
			err = reply("250 Accepted")
			if scenario == "accepted-disconnect" {
				return err
			}
		default:
			return errors.New("unexpected SMTP fixture command")
		}
		if err != nil {
			return err
		}
	}
}

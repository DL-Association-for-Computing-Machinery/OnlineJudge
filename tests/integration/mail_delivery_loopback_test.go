//go:build integration

package integration_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailmessage"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/maildelivery"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailtransport"
)

func TestAccountMailDeliveryLoopback(t *testing.T) {
	f := newBacklogFixture(t)
	must(t, mailqueue.MigrateUp(f.ctx, f.a))
	queue, err := mailqueue.NewEnqueuer(f.ctx, f.a)
	must(t, err)
	h := identityHasher(t, false)
	issuer, err := identitypg.New(f.a, h)
	must(t, err)
	to, err := identity.ParseEmail("Mailbox+tag@example.invalid")
	must(t, err)
	from, err := identity.ParseEmail("sender@example.invalid")
	must(t, err)
	subject := identityID(1)
	record := identityRecord(t, h, "synthetic loopback delivery password")
	_, err = issuer.InitializePersonal(f.ctx, subject, to, true, record)
	must(t, err)
	epoch, err := issuer.BeginRecovery(f.ctx)
	must(t, err)
	must(t, issuer.FinishRecovery(f.ctx, epoch))
	protector, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	id, err := issuer.IssueEmailToken(f.ctx, subject, identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch},
		emailtoken.Activation, protector, func(context.Context, string) error { return nil }, queue.Enqueue)
	must(t, err)
	var queued string
	must(t, f.b.QueryRow(f.ctx, `SELECT args->>'payload_id' FROM river_job WHERE queue=$1`, mailqueue.Queue).Scan(&queued))
	if queued != id {
		t.Fatal("River did not receive the issued payload ID")
	}
	reader, err := identitypg.NewEmailDeliveryReader(f.b, protector)
	must(t, err)
	renderer, err := emailmessage.New("https://oj.example.invalid")
	must(t, err)
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	tlsConfig := &tls.Config{Certificates: fixture.TLS.Certificates, MinVersion: tls.VersionTLS12}
	roots := x509.NewCertPool()
	roots.AddCert(fixture.Certificate())
	fixture.Close()

	var original string
	for _, want := range []maildelivery.Outcome{maildelivery.Unknown, maildelivery.Accepted} {
		listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
		must(t, err)
		t.Cleanup(func() { _ = listener.Close() })
		must(t, listener.SetDeadline(time.Now().Add(5*time.Second)))
		type result struct {
			token string
			err   error
		}
		received := make(chan result, 1)
		go func() {
			token, err := readAccountSMTP(listener, tlsConfig, to, want == maildelivery.Accepted)
			received <- result{token, err}
		}()
		sender, err := mailtransport.New(mailtransport.Config{Host: "127.0.0.1",
			Port: listener.Addr().(*net.TCPAddr).Port, TLS: mailtransport.ImplicitTLS,
			From: from, Timeout: 5 * time.Second, RootCAs: roots}, "", "")
		must(t, err)
		attempt, err := maildelivery.New(reader, renderer, sender.Send)
		must(t, err)
		if got := attempt.Run(f.ctx, queued); got != want {
			t.Fatalf("loopback delivery result = %d, want %d", got, want)
		}
		data := <-received
		must(t, data.err)
		if original != "" && original != data.token {
			t.Fatal("unknown-result retry sent a different credential")
		}
		original = data.token
	}
	token, err := emailtoken.Parse(original)
	must(t, err)
	must(t, issuer.ActivateEmail(f.ctx, token, record))
	personal, err := issuer.PersonalByID(f.ctx, subject)
	must(t, err)
	if !personal.EmailVerified {
		t.Fatal("the credential delivered over SMTP did not activate the account")
	}
}

func readAccountSMTP(listener *net.TCPListener, config *tls.Config, to identity.Email, acknowledge bool) (string, error) {
	raw, err := listener.AcceptTCP()
	if err != nil {
		return "", err
	}
	defer func() { _ = raw.Close() }()
	if err := raw.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return "", err
	}
	conn := tls.Server(raw, config)
	if err := conn.Handshake(); err != nil {
		return "", err
	}
	reader := textproto.NewReader(bufio.NewReader(conn))
	reply := func(line string) error { _, err := fmt.Fprintf(conn, "%s\r\n", line); return err }
	if err := reply("220 fixture.example.invalid ESMTP"); err != nil {
		return "", err
	}
	var token string
	var recipient bool
	for {
		line, err := reader.ReadLine()
		if err != nil {
			return "", err
		}
		switch {
		case strings.HasPrefix(line, "EHLO "), strings.HasPrefix(line, "MAIL FROM:"), line == "NOOP":
			err = reply("250 OK")
		case line == "RCPT TO:<"+to.Original()+">":
			recipient = true
			err = reply("250 OK")
		case line == "DATA" && recipient && token == "":
			if err := reply("354 Continue"); err != nil {
				return "", err
			}
			token, err = readActivationLink(reader.DotReader())
			if err != nil || !acknowledge {
				return token, err
			}
			err = reply("250 Accepted")
		case line == "RSET" && token != "":
			return token, reply("250 OK")
		default:
			return "", errors.New("unexpected account SMTP fixture command")
		}
		if err != nil {
			return "", err
		}
	}
}

func readActivationLink(data io.Reader) (string, error) {
	message, err := mail.ReadMessage(bufio.NewReader(io.LimitReader(data, 16384)))
	if err != nil {
		return "", errors.New("invalid fixture message headers")
	}
	bodyReader := message.Body
	if strings.EqualFold(message.Header.Get("Content-Transfer-Encoding"), "quoted-printable") {
		bodyReader = quotedprintable.NewReader(bodyReader)
	}
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return "", errors.New("invalid fixture message body")
	}
	const prefix = "https://oj.example.invalid/account/activate#token="
	for line := range strings.SplitSeq(string(body), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSuffix(line, "\r"), prefix); ok {
			if _, err := emailtoken.Parse(value); err == nil {
				return value, nil
			}
		}
	}
	return "", errors.New("fixture message did not contain a valid activation link")
}

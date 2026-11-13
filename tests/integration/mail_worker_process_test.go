//go:build integration

package integration_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
)

func TestMailWorkerProcessLoopback(t *testing.T) {
	f := newBacklogFixture(t)
	must(t, mailqueue.MigrateUp(f.ctx, f.a))
	queue, err := mailqueue.NewEnqueuer(f.ctx, f.a)
	must(t, err)
	h := identityHasher(t, false)
	issuer, err := identitypg.New(f.a, h)
	must(t, err)
	to, err := identity.ParseEmail("Mailbox+tag@example.invalid")
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
	_, err = f.a.Exec(f.ctx, "INSERT INTO account_mail_send_rate(spacing_ms) VALUES (100)")
	must(t, err)
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	tlsConfig := &tls.Config{Certificates: fixture.TLS.Certificates, MinVersion: tls.VersionTLS12}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: fixture.Certificate().Raw})
	fixture.Close()
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "local-ca.pem"), filepath.Join(dir, "payload.json")
	must(t, os.WriteFile(certFile, certPEM, 0600))
	key := [32]byte{1}
	keyJSON := fmt.Sprintf(`{"format":"oj.email-payload-keys.v1","current":"fixture","keys":{"fixture":%q}}`,
		base64.RawURLEncoding.EncodeToString(key[:]))
	must(t, os.WriteFile(keyFile, []byte(keyJSON), 0600))
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	must(t, err)
	defer func() { _ = listener.Close() }()
	must(t, listener.SetDeadline(time.Now().Add(14*time.Second)))
	type reception struct {
		value string
		err   error
	}
	received := make(chan reception, 2)
	go func() {
		for _, ack := range []bool{false, true} {
			value, err := readAccountSMTP(listener, tlsConfig, to, ack)
			received <- reception{value, err}
			if err != nil {
				return
			}
		}
	}()
	var schema string
	must(t, f.a.QueryRow(f.ctx, "SELECT current_schema()").Scan(&schema))
	path, err := filepath.Abs("../../.local/bin/mail-worker")
	must(t, err)
	process, cancel := context.WithCancel(f.ctx)
	defer cancel()
	cmd := exec.CommandContext(process, path)
	cmd.Env = []string{
		"OJ_DATABASE_URL=" + f.a.Config().ConnString() + " search_path=" + schema,
		"OJ_EMAIL_PAYLOAD_KEYS_FILE=" + keyFile, "SSL_CERT_FILE=" + certFile,
		"OJ_ACCOUNT_PUBLIC_ORIGIN=https://oj.example.invalid",
		"OJ_SMTP_HOST=127.0.0.1", "OJ_SMTP_TLS=implicit", "OJ_SMTP_FROM=sender@example.invalid",
		"OJ_SMTP_PORT=" + strconv.Itoa(listener.Addr().(*net.TCPAddr).Port),
	}
	must(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() { cancel() })
	next := func() string {
		t.Helper()
		select {
		case data := <-received:
			must(t, data.err)
			return data.value
		case err := <-done:
			t.Fatalf("mail process exited early: %v", err)
		case <-f.ctx.Done():
			t.Fatal("mail process did not deliver")
		}
		return ""
	}
	first := next()
	if next() != first {
		t.Fatal("automatic retry changed the activation credential")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var state, result string
		var attempt int
		var payload bool
		must(t, f.a.QueryRow(f.ctx, `SELECT state::text, attempt, COALESCE(metadata->>'oj_delivery_result',''),
			EXISTS(SELECT 1 FROM identity_email_payloads WHERE token_id=$1)
			FROM river_job WHERE args->>'payload_id'=$1`, id).Scan(&state, &attempt, &result, &payload))
		if state == "completed" && result == "accepted" && attempt == 2 && !payload {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("acknowledged delivery did not complete atomically")
		}
		time.Sleep(20 * time.Millisecond)
	}
	token, err := emailtoken.Parse(first)
	must(t, err)
	must(t, issuer.ActivateEmail(f.ctx, token, record))
	must(t, cmd.Process.Signal(syscall.SIGTERM))
	select {
	case err := <-done:
		must(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("mail process did not stop gracefully")
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

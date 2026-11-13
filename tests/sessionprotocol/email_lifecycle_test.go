//go:build integration

package sessionprotocol_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
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
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailmessage"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailbudget"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/maildelivery"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailtransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/emailintake"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

func TestIsolatedEmailLifecycle(t *testing.T) {
	if os.Getenv("OJ_ISOLATED_EMAIL_LIFECYCLE") != "1" {
		t.Skip("requires its own fresh protocol fixture")
	}
	f := setup(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	must(t, mailqueue.MigrateUp(ctx, f.pools[0]))
	must(t, mailqueue.ConfigureSendRate(ctx, f.pools[0], 100))
	_, err := f.pools[0].Exec(ctx, "CREATE DATABASE fixture_email_seals")
	must(t, err)
	ledgerConfig := f.pools[0].Config()
	ledgerConfig.ConnConfig.Database = "fixture_email_seals"
	ledger, err := pgxpool.NewWithConfig(ctx, ledgerConfig)
	must(t, err)
	t.Cleanup(ledger.Close)
	keys, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": loginKey(t)})
	must(t, err)
	must(t, mailbudget.Initialize(ctx, ledger))
	must(t, mailbudget.Provision(ctx, ledger, keys, 2))

	c := runtimeConfig(t, f, namespace(t), true)
	bucket := protocol.LoginBucket{Count: 30, Window: time.Hour}
	c.EmailLimits = &protocol.EmailLimits{RequestGlobal: bucket, RequestSource: bucket, RequestMailbox: bucket,
		ConfirmGlobal: bucket, ConfirmSource: bucket, ConfirmToken: bucket}
	weak := sha256.Sum256([]byte("synthetic blocked password"))
	c.EmailConfirmation, err = password.NewAcceptance(15, c.PasswordLimits.MaxPasswordBytes, [][32]byte{weak})
	must(t, err)
	secrets, err := protocol.LoadLoginSecrets(c.SecretsFile)
	must(t, err)
	must(t, protocol.InitializeEmailLimits(ctx, f.store(t, c.Namespace, 0, protocol.OrdinaryPolicy()), secrets, *c.EmailLimits))

	const register, recover = "/api/v1/account/register", "/api/v1/account/request-password-reset"
	const activate, reset = "/api/v1/account/activate", "/api/v1/account/reset-password"
	const address = "isolated-lifecycle@example.invalid"
	const signupBody = `{"email":"` + address + `"}`
	disabled := loginHandlerServer(t, httpapi.NewSessionHandler(openRuntime(t, c)))
	loginStatus(t, loginCall(disabled, register, signupBody, nil, nil), 404)
	c.IsolatedEmailIntake = &sessionruntime.IsolatedEmailIntakeConfig{
		Limits: emailintake.Config{Capacity: 4, Workers: 1, MaxAge: 5 * time.Second}, Protector: keys, Ledger: ledger,
	}
	bad := c
	sameDatabase := *c.IsolatedEmailIntake
	sameDatabase.Ledger = f.pools[0]
	bad.IsolatedEmailIntake = &sameDatabase
	if r, err := sessionruntime.Open(ctx, bad); err == nil {
		must(t, r.Close(ctx))
		t.Fatal("email intake accepted the application database as a seal ledger")
	}
	c.Database.ApplicationName = "isolated_email_api_a"
	c.Database.Timeout = 2 * time.Second
	other := c
	other.Database.ApplicationName = "isolated_email_api_b"
	apiA, apiB := openRuntime(t, c), openRuntime(t, other)
	x := loginHandlerServer(t, httpapi.NewSessionHandler(apiA))
	y := loginHandlerServer(t, httpapi.NewSessionHandler(apiB))
	loginStatus(t, loginCall(x, register, signupBody, nil, func(r *http.Request) {
		r.Header.Set("Origin", "https://other.example.invalid")
	}), 403)
	to, err := identity.ParseEmail(address)
	must(t, err)
	from, err := identity.ParseEmail("sender@example.invalid")
	must(t, err)
	smtp, receive := lifecycleSMTP(t, from, to, []string{activate, reset})
	renderer, err := emailmessage.New(loginOrigin)
	must(t, err)
	worker, err := maildelivery.NewWorker(ctx, f.pools[1], keys, renderer, smtp)
	must(t, err)
	sendCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- worker.Run(sendCtx) }()
	t.Cleanup(func() {
		stop()
		select {
		case err := <-done:
			must(t, err)
		case <-time.After(6 * time.Second):
			t.Error("mail worker failed to stop")
		}
	})

	request := func(e *loginEndpoint, path, body string) {
		t.Helper()
		r := loginCall(e, path, body, nil, nil)
		loginStatus(t, r, 202)
		if len(r.body) != 0 || len(r.cookies) != 0 || r.headers.Get("Cache-Control") != "no-store" {
			t.Fatal("request exposed outcome or issued session")
		}
	}
	confirm := func(e *loginEndpoint, path, token, newPassword string, want int) {
		t.Helper()
		body, err := json.Marshal(map[string]string{"token": token, "new_password": newPassword})
		must(t, err)
		r := loginCall(e, path, string(body), nil, nil)
		if r.err != nil || r.status != want || len(r.cookies) != 0 {
			t.Fatalf("confirmation status = %d; want %d, with no cookie", r.status, want)
		}
	}
	checkSession := func(e *loginEndpoint, credential protocol.Credential, want int) {
		t.Helper()
		loginStatus(t, loginCall(e, "/api/v1/session", "", credentialCookies(credential), func(r *http.Request) {
			r.Method = http.MethodGet
		}), want)
	}
	const first = "synthetic isolated activation password"
	const second = "synthetic isolated replacement password"
	request(x, register, signupBody)
	token := receive(ctx)
	waitLifecycleMail(t, ctx, f.pools[0], 1)
	p, err := f.identities[0].LookupPersonalByEmail(ctx, to)
	must(t, err)
	if p.EmailVerified {
		t.Fatal("registration activated before link confirmation")
	}
	confirm(y, activate, token, first, 204)
	confirm(x, activate, token, first, 400)
	old := entryCredential(t, x, p, first)
	checkSession(y, old, 200)
	request(y, recover, `{"email":"missing-isolated@example.invalid"}`)
	request(y, recover, signupBody)
	replacement := receive(ctx)
	waitLifecycleMail(t, ctx, f.pools[0], 2)
	checkSession(y, old, 200)
	confirm(x, reset, replacement, "synthetic blocked password", 400)
	checkSession(y, old, 200)
	confirm(x, reset, replacement, second, 204)
	confirm(y, reset, replacement, second, 400)
	checkSession(x, old, 401)
	checkSession(y, old, 401)
	body, err := json.Marshal(map[string]string{"email": address, "password": first})
	must(t, err)
	r := loginCall(x, "/api/v1/session/login/begin", string(body), nil, nil)
	if r.err != nil || r.status != 401 {
		t.Fatalf("old password status = %d; want 401", r.status)
	}
	checkSession(x, entryCredential(t, y, p, second), 200)
	// Move the fixture timestamps together past the resend interval to
	// distinguish the independent budget from the normal resend gate.
	_, err = f.pools[0].Exec(ctx, `UPDATE identity_email_tokens
		SET issued_at=issued_at-interval '61 seconds', expires_at=expires_at-interval '61 seconds'
		WHERE subject_id=$1 AND purpose=$2`, p.ID, string(emailtoken.Recovery))
	must(t, err)
	hold, err := f.pools[0].Begin(ctx)
	must(t, err)
	defer func() { _ = hold.Rollback(ctx) }()
	var holder int32
	must(t, hold.QueryRow(ctx, "SELECT pg_backend_pid() FROM identity_credentials WHERE id=$1 FOR UPDATE", p.ID).Scan(&holder))
	request(x, recover, signupBody)
	waitLifecycleIntakeBlocked(t, ctx, f.pools[0], c.Database.ApplicationName, holder)
	short, stopClose := context.WithTimeout(ctx, 30*time.Millisecond)
	err = apiA.Close(short)
	stopClose()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("API closed before draining the blocked background request")
	}
	loginStatus(t, loginCall(x, recover, signupBody, nil, nil), 503)
	must(t, hold.Rollback(ctx))
	must(t, apiA.Close(ctx))
	must(t, apiB.Close(ctx))
	var charged, jobs, tokens int
	must(t, ledger.QueryRow(ctx, "SELECT used FROM public.oj_mail_seal_budget").Scan(&charged))
	must(t, f.pools[0].QueryRow(ctx, "SELECT count(*) FROM river_job").Scan(&jobs))
	must(t, f.pools[0].QueryRow(ctx, "SELECT count(*) FROM identity_email_tokens WHERE subject_id=$1", p.ID).Scan(&tokens))
	if charged != 2 || jobs != 2 || tokens != 2 {
		t.Fatal("mail issuance diverged from the independent budget")
	}
}

func waitLifecycleIntakeBlocked(t *testing.T, ctx context.Context, pool *pgxpool.Pool, application string, holder int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		var blocked bool
		must(t, pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND $2=ANY(pg_blocking_pids(pid))
		)`, application, holder).Scan(&blocked))
		if blocked {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("background issuance never reached the locked account")
		case <-tick.C:
		}
	}
}

func waitLifecycleMail(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int) {
	t.Helper()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		var completed, payloads int
		must(t, pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM river_job WHERE state='completed'),
			(SELECT count(*) FROM identity_email_payloads)`).Scan(&completed, &payloads))
		if completed == want && payloads == 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("mail did not complete and retire the encrypted payload")
		case <-tick.C:
		}
	}
}

func lifecycleSMTP(t *testing.T, from, to identity.Email, links []string) (*mailtransport.Sender, func(context.Context) string) {
	t.Helper()
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	config := &tls.Config{Certificates: fixture.TLS.Certificates, MinVersion: tls.VersionTLS12}
	roots := x509.NewCertPool()
	roots.AddCert(fixture.Certificate())
	fixture.Close()
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	must(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	sender, err := mailtransport.New(mailtransport.Config{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port,
		TLS: mailtransport.ImplicitTLS, From: from, Timeout: 3 * time.Second, RootCAs: roots}, "", "")
	must(t, err)
	type result struct {
		token string
		err   error
	}
	inbox := make(chan result, len(links))
	go func() {
		for _, link := range links {
			token, err := readLifecycleSMTP(listener, config, to, loginOrigin+strings.TrimPrefix(link, "/api/v1")+"#token=")
			inbox <- result{token, err}
			if err != nil {
				return
			}
		}
	}()
	return sender, func(ctx context.Context) string {
		t.Helper()
		select {
		case message := <-inbox:
			if message.err != nil {
				t.Fatal("local SMTP failed to receive the expected confirmation link")
			}
			return message.token
		case <-ctx.Done():
			t.Fatal("timed out waiting for local SMTP")
			return ""
		}
	}
}

func readLifecycleSMTP(listener *net.TCPListener, config *tls.Config, to identity.Email, prefix string) (string, error) {
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
	if err := reply("220 isolated.example.invalid ESMTP"); err != nil {
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
			if err = reply("354 Continue"); err != nil {
				return "", err
			}
			token, err = lifecycleLink(reader.DotReader(), prefix)
			if err == nil {
				err = reply("250 Accepted")
			}
		case line == "RSET" && token != "":
			return token, reply("250 OK")
		default:
			return "", errors.New("unexpected fixture SMTP command")
		}
		if err != nil {
			return "", err
		}
	}
}

func lifecycleLink(r io.Reader, prefix string) (string, error) {
	message, err := mail.ReadMessage(bufio.NewReader(io.LimitReader(r, 16384)))
	if err != nil {
		return "", err
	}
	body := message.Body
	if strings.EqualFold(message.Header.Get("Content-Transfer-Encoding"), "quoted-printable") {
		body = quotedprintable.NewReader(body)
	}
	content, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(content), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSuffix(line, "\r"), prefix); ok {
			if _, err := emailtoken.Parse(value); err == nil {
				return value, nil
			}
		}
	}
	return "", errors.New("expected fixture link missing")
}

//go:build integration

package sessionprotocol_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailbudget"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/emailintake"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func TestIsolatedEmailLifecycle(t *testing.T) {
	if os.Getenv("OJ_ISOLATED_EMAIL_LIFECYCLE") != "1" {
		t.Skip("requires its own fresh protocol fixture")
	}
	f := setup(t)
	binary := entryBinary(t)
	workerBinary := entryProgram(t, "mail-worker")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	_, err := f.pools[0].Exec(ctx, "CREATE DATABASE fixture_email_seals")
	must(t, err)
	ledgerConfig := f.pools[0].Config()
	ledgerConfig.ConnConfig.Database = "fixture_email_seals"
	ledger, err := pgxpool.NewWithConfig(ctx, ledgerConfig)
	must(t, err)
	t.Cleanup(ledger.Close)
	material := loginKey(t)
	keyFile := writeLifecycleJSON(t, "mail-keys.json", map[string]any{
		"format": "oj.email-payload-keys.v1", "current": "fixture",
		"keys": map[string]string{"fixture": base64.RawURLEncoding.EncodeToString(material[:])},
	})
	keys, err := emailtoken.LoadProtector(keyFile)
	must(t, err)

	c := runtimeConfig(t, f, namespace(t), true)
	bucket := protocol.LoginBucket{Count: 30, Window: time.Hour}
	c.EmailLimits = &protocol.EmailLimits{RequestGlobal: bucket, RequestSource: bucket, RequestMailbox: bucket,
		ConfirmGlobal: bucket, ConfirmSource: bucket, ConfirmToken: bucket}
	weak := sha256.Sum256([]byte("synthetic blocked password"))
	blocklist := writeLifecycleJSON(t, "weak-passwords.json", map[string]any{"version": 1, "sha256": []string{hex.EncodeToString(weak[:])}})
	c.EmailConfirmation, err = password.NewAcceptance(15, c.PasswordLimits.MaxPasswordBytes, [][32]byte{weak})
	must(t, err)
	secrets, err := protocol.LoadLoginSecrets(c.SecretsFile)
	must(t, err)
	must(t, protocol.InitializeEmailLimits(ctx, f.store(t, c.Namespace, 0, protocol.OrdinaryPolicy()), secrets, *c.EmailLimits))

	const register, recover = "/api/v1/account/register", "/api/v1/account/request-password-reset"
	const activate, reset = "/api/v1/account/activate", "/api/v1/account/reset-password"
	const address = "isolated-lifecycle@example.invalid"
	const signupBody = `{"email":"` + address + `"}`
	ca := testpki.New(t)
	const shutdown = 4 * time.Second
	disabled := entryStart(t, binary, entryConfig(t, c, ca, shutdown, blocklist), "127.0.0.1:0")
	endpoint := entryClient(t, disabled, ca)
	loginStatus(t, loginCall(endpoint, register, signupBody, nil, nil), 404)
	loginStatus(t, loginCall(endpoint, recover, signupBody, nil, nil), 404)
	disabled.signal(t)
	disabled.wait(t, true)
	c.Database.ApplicationName = "isolated_email_api_a"
	c.Database.Timeout = 2 * time.Second
	budgetDB := c.Database
	budgetDB.Database, budgetDB.ApplicationName, budgetDB.Connections = "fixture_email_seals", "isolated_email_budget_a", 1
	c.EmailIntake = &sessionruntime.EmailIntakeConfig{
		Limits:         emailintake.Config{Capacity: 4, Workers: 1, MaxAge: 5 * time.Second},
		PayloadKeyFile: keyFile, BudgetDatabase: budgetDB,
	}
	refuse := func(cfg sessionruntime.Config) {
		t.Helper()
		proc := entryStart(t, binary, entryConfig(t, cfg, ca, shutdown, blocklist), "127.0.0.1:0")
		proc.wait(t, false)
		if _, ok := proc.event("API starting"); ok {
			t.Fatal("API opened a listener with invalid email intake dependencies")
		}
	}
	refuse(c) // No ledger or active-key counter.
	must(t, mailbudget.Initialize(ctx, ledger))
	must(t, mailbudget.Provision(ctx, ledger, keys, 3))
	refuse(c) // Ledger ready, but River schema is absent.
	must(t, mailqueue.MigrateUp(ctx, f.pools[0]))
	must(t, mailqueue.ConfigureSendRate(ctx, f.pools[0], 100))
	badConfig, badIntake := c, *c.EmailIntake
	badConfig.EmailIntake = &badIntake
	badIntake.BudgetDatabase.Database = c.Database.Database
	refuse(badConfig)
	badIntake.BudgetDatabase = budgetDB
	badIntake.PayloadKeyFile = writeLifecycleJSON(t, "bad-mail-keys.json", map[string]any{"format": "invalid"})
	refuse(badConfig)
	other := c
	other.Database.ApplicationName = "isolated_email_api_b"
	otherIntake := *c.EmailIntake
	otherIntake.BudgetDatabase.ApplicationName = "isolated_email_budget_b"
	other.EmailIntake = &otherIntake
	apiA := entryStart(t, binary, entryConfig(t, c, ca, shutdown, blocklist), "127.0.0.1:0")
	apiB := entryStart(t, binary, entryConfig(t, other, ca, shutdown, blocklist), "127.0.0.1:0")
	x := entryClient(t, apiA, ca)
	y := entryClient(t, apiB, ca)
	loginStatus(t, loginCall(x, register, signupBody, nil, func(r *http.Request) {
		r.Header.Set("Origin", "https://other.example.invalid")
	}), 403)
	to, err := identity.ParseEmail(address)
	must(t, err)
	from, err := identity.ParseEmail("sender@example.invalid")
	must(t, err)
	port, trustFile, receive := lifecycleSMTP(t, to, []string{activate, reset, reset})
	startLifecycleWorker(t, workerBinary, os.Getenv("OJ_PROTOCOL_DATABASE_URL"), keyFile, trustFile, from, port)

	request := func(e *loginEndpoint, path, body string) {
		t.Helper()
		r := loginCall(e, path, body, nil, nil)
		loginStatus(t, r, 202)
		if len(r.body) != 0 || len(r.cookies) != 0 || r.headers.Get("Cache-Control") != "no-store" {
			t.Fatal("request exposed outcome or issued session")
		}
	}
	confirmationBody := func(token, newPassword string) string {
		t.Helper()
		body, err := json.Marshal(map[string]string{"token": token, "new_password": newPassword})
		must(t, err)
		return string(body)
	}
	confirm := func(e *loginEndpoint, path, token, newPassword string, want int) {
		t.Helper()
		r := loginCall(e, path, confirmationBody(token, newPassword), nil, nil)
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
	// Hold the account row until both API transactions reach the consume lock.
	// Starting both requests together alone would also allow a sequential run.
	resetHold, resetHolder := holdLifecycleAccount(t, ctx, f.pools[0], p.ID)
	passwords := [2]string{"synthetic isolated replacement alpha", "synthetic isolated replacement beta"}
	endpoints := [2]*loginEndpoint{x, y}
	type confirmationResult struct {
		index    int
		response loginResponse
	}
	results := make(chan confirmationResult, len(endpoints))
	for i, endpoint := range endpoints {
		body := confirmationBody(replacement, passwords[i])
		go func() {
			results <- confirmationResult{i, loginCall(endpoint, reset, body, nil, nil)}
		}()
	}
	waitLifecycleAccountBlocked(t, ctx, f.pools[0], resetHolder, c.Database.ApplicationName, other.Database.ApplicationName)
	must(t, resetHold.Rollback(ctx))
	winner := -1
	for range endpoints {
		result := <-results
		want := 400
		if result.response.status == 204 && winner == -1 {
			winner = result.index
			want = 204
		}
		loginStatus(t, result.response, want)
		if len(result.response.headers.Values("Set-Cookie")) != 0 {
			t.Fatal("reset confirmation changed the session cookie")
		}
	}
	if winner == -1 {
		t.Fatal("neither API confirmed the password reset")
	}
	checkSession(x, old, 401)
	checkSession(y, old, 401)
	for _, rejected := range []string{first, passwords[1-winner]} {
		body, err := json.Marshal(map[string]string{"email": address, "password": rejected})
		must(t, err)
		loginStatus(t, loginCall(x, "/api/v1/session/login/begin", string(body), nil, nil), 401)
	}
	checkSession(x, entryCredential(t, y, p, passwords[winner]), 200)
	// Move both timestamps past the resend interval so the in-flight request
	// can prove the budget pool stays open through graceful shutdown.
	_, err = f.pools[0].Exec(ctx, `UPDATE identity_email_tokens
		SET issued_at=issued_at-interval '61 seconds', expires_at=expires_at-interval '61 seconds'
		WHERE subject_id=$1 AND purpose=$2`, p.ID, string(emailtoken.Recovery))
	must(t, err)
	hold, holder := holdLifecycleAccount(t, ctx, f.pools[0], p.ID)
	request(x, recover, signupBody)
	waitLifecycleAccountBlocked(t, ctx, f.pools[0], holder, c.Database.ApplicationName)
	apiA.signal(t)
	apiA.waitEvent(t, "API stopping")
	entryListenerClosed(t, apiA.waitEvent(t, "API starting").Address)
	waitLifecycleAccountBlocked(t, ctx, f.pools[0], holder, c.Database.ApplicationName)
	select {
	case <-apiA.done:
		t.Fatal("API exited before draining a blocked email request")
	case <-time.After(30 * time.Millisecond):
		if _, ok := apiA.event("API stopped gracefully"); ok {
			t.Fatal("API reported graceful shutdown while email work was blocked")
		}
	}
	waitLifecycleAccountBlocked(t, ctx, f.pools[0], holder, c.Database.ApplicationName)
	must(t, hold.Rollback(ctx))
	apiB.signal(t)
	for _, proc := range []*entryProcess{apiA, apiB} {
		proc.wait(t, true)
		if _, ok := proc.event("API stopped gracefully"); !ok {
			t.Fatal("API did not confirm a graceful shutdown")
		}
	}
	_ = receive(ctx)
	waitLifecycleMail(t, ctx, f.pools[0], 3)
	refuse(c) // A configured but exhausted ledger must not be treated as ready.
	var charged, jobs, tokens int
	must(t, ledger.QueryRow(ctx, "SELECT used FROM public.oj_mail_seal_budget").Scan(&charged))
	must(t, f.pools[0].QueryRow(ctx, "SELECT count(*) FROM river_job").Scan(&jobs))
	must(t, f.pools[0].QueryRow(ctx, "SELECT count(*) FROM identity_email_tokens WHERE subject_id=$1", p.ID).Scan(&tokens))
	if charged != 3 || jobs != 3 || tokens != 3 {
		t.Fatal("mail issuance diverged from the independent budget")
	}
}

func writeLifecycleJSON(t *testing.T, name string, data map[string]any) string {
	t.Helper()
	body, err := json.Marshal(data)
	must(t, err)
	path := filepath.Join(t.TempDir(), name)
	must(t, os.WriteFile(path, body, 0600))
	return entryPath(t, path)
}

func holdLifecycleAccount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string) (pgx.Tx, int32) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	must(t, err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	})
	var holder int32
	must(t, tx.QueryRow(ctx, "SELECT pg_backend_pid() FROM identity_credentials WHERE id=$1 FOR UPDATE", id).Scan(&holder))
	return tx, holder
}

func waitLifecycleAccountBlocked(t *testing.T, ctx context.Context, pool *pgxpool.Pool, holder int32, applications ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		var blocked int
		// A second waiter can be queued behind the first as well as the holder.
		must(t, pool.QueryRow(ctx, `WITH RECURSIVE waiting(pid, application_name, blockers) AS MATERIALIZED (
			SELECT pid, application_name, pg_blocking_pids(pid) FROM pg_stat_activity WHERE application_name=ANY($1)
		), blocked(pid) AS (
			SELECT $2::int UNION SELECT w.pid FROM waiting w JOIN blocked b ON b.pid=ANY(w.blockers)
		) SELECT count(DISTINCT w.application_name) FROM waiting w JOIN blocked b USING (pid)`, applications, holder).Scan(&blocked))
		if blocked == len(applications) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("not all API transactions reached the locked account")
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

func lifecycleSMTP(t *testing.T, to identity.Email, links []string) (int, string, func(context.Context) string) {
	t.Helper()
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	config := &tls.Config{Certificates: fixture.TLS.Certificates, MinVersion: tls.VersionTLS12}
	trustFile := filepath.Join(t.TempDir(), "receipt-fixture.pem")
	must(t, os.WriteFile(trustFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: fixture.Certificate().Raw}), 0600))
	fixture.Close()
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	must(t, err)
	t.Cleanup(func() { _ = listener.Close() })
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
	return listener.Addr().(*net.TCPAddr).Port, trustFile, func(ctx context.Context) string {
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

func startLifecycleWorker(t *testing.T, workerBinary, databaseURL, keyFile, trustFile string, from identity.Email, port int) {
	t.Helper()
	cmd := exec.Command(workerBinary)
	cmd.Env = []string{
		"OJ_DATABASE_URL=" + databaseURL,
		"OJ_EMAIL_PAYLOAD_KEYS_FILE=" + keyFile, "SSL_CERT_FILE=" + trustFile,
		"OJ_ACCOUNT_PUBLIC_ORIGIN=" + loginOrigin,
		"OJ_SMTP_HOST=127.0.0.1", "OJ_SMTP_PORT=" + strconv.Itoa(port),
		"OJ_SMTP_TLS=implicit", "OJ_SMTP_FROM=" + from.Original(),
	}
	must(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		t.Helper()
		select {
		case err := <-done:
			t.Errorf("mail worker exited before shutdown: %v", err)
			return
		default:
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Error("mail worker could not be signalled")
		}
		select {
		case err := <-done:
			must(t, err)
		case <-time.After(6 * time.Second):
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Errorf("could not stop mail worker: %v", err)
				return
			}
			t.Errorf("mail worker exceeded its shutdown deadline (wait result: %v)", <-done)
		}
	})
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

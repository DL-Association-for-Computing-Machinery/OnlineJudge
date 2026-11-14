//go:build integration

package sessionprotocol_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailbudget"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/emailintake"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func TestIsolatedEmailArchiveRecovery(t *testing.T) {
	if os.Getenv("OJ_ISOLATED_EMAIL_ARCHIVE_RECOVERY") != "1" {
		t.Skip("requires its own fresh protocol fixture")
	}
	f := setup(t)
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	binary := entryBinary(t)
	worker := entryProgram(t, "mail-worker")
	c := runtimeConfig(t, f, namespace(t), true)
	ledger, keys, blocklist := configureArchiveMail(t, ctx, f, &c)
	checkCharges := func(want int) {
		t.Helper()
		var used int
		must(t, ledger.QueryRow(ctx, "SELECT used FROM public.oj_mail_seal_budget").Scan(&used))
		if used != want {
			t.Fatalf("seal charges = %d, want %d", used, want)
		}
	}
	ca := testpki.New(t)
	start := func(cfg sessionruntime.Config) *entryProcess {
		t.Helper()
		return entryStart(t, binary, entryConfig(t, cfg, ca, 4*time.Second, blocklist), "127.0.0.1:0")
	}
	register := func(endpoint *loginEndpoint, address string) {
		t.Helper()
		loginStatus(t, loginCall(endpoint, "/api/v1/account/register", `{"email":"`+address+`"}`, nil, nil), http.StatusAccepted)
	}
	const oldAddress = "archive-old@example.invalid"
	source := start(c)
	x := entryClient(t, source, ca)
	register(x, oldAddress)
	oldID := waitArchiveMailJob(t, ctx, f.pools[0])
	checkCharges(1)
	archive := filepath.Join(t.TempDir(), "postgres.dump")
	dumpEntryDatabase(t, ctx, c.Database, archive)
	register(x, "archive-after-backup@example.invalid")
	// Drain the asynchronous registration before restoring the earlier archive.
	source.signal(t)
	source.wait(t, true)
	var count int
	must(t, f.pools[0].QueryRow(ctx, "SELECT count(*) FROM river_job").Scan(&count))
	if count != 2 {
		t.Fatal("post-backup issuance did not commit before restore")
	}
	checkCharges(2)

	c.Database = restoreEntryDatabase(t, ctx, c.Database, archive)
	url := "host=" + c.Database.Host + " port=" + strconv.Itoa(int(c.Database.Port)) + " user=oj_protocol dbname=postgres sslmode=disable"
	target, err := pgxpool.New(ctx, url)
	must(t, err)
	t.Cleanup(target.Close)
	reader, err := identitypg.NewEmailDeliveryReader(target, keys)
	must(t, err)
	archived, err := reader.Prepare(ctx, oldID)
	must(t, err)
	if archived.Token().LinkValue() == "" {
		t.Fatal("archive did not contain the original live payload")
	}
	authority, err := identitypg.New(target, f.hasher)
	must(t, err)
	generation, err := authority.BeginRecovery(ctx)
	must(t, err)
	must(t, authority.FinishRecovery(ctx, generation))

	const freshAddress = "archive-fresh@example.invalid"
	to, err := identity.ParseEmail(freshAddress)
	must(t, err)
	from, err := identity.ParseEmail("sender@example.invalid")
	must(t, err)
	const activate = "/api/v1/account/activate"
	port, trustFile, receive := lifecycleSMTPWithRetry(t, to, []string{activate}, true)
	permit := func() string {
		t.Helper()
		var next string
		must(t, target.QueryRow(ctx, "SELECT next_permit_at::text FROM account_mail_send_rate WHERE singleton").Scan(&next))
		return next
	}
	before := permit()
	startLifecycleWorker(t, worker, url, c.EmailIntake.PayloadKeyFile, trustFile, from, port)
	waitLifecycleMail(t, ctx, target, 1)
	var obsolete bool
	must(t, target.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM river_job WHERE args->>'payload_id'=$1
		AND state='completed' AND metadata->>'oj_delivery_result'='obsolete')
		AND EXISTS (SELECT 1 FROM identity_email_tokens WHERE id=$1)`, oldID).Scan(&obsolete))
	if !obsolete || permit() != before {
		t.Fatal("restored job was not retired before acquiring an SMTP permit")
	}
	checkCharges(2)

	a := start(c)
	c.Database.ApplicationName += "_replica"
	b := start(c)
	register(entryClient(t, a, ca), freshAddress)
	waitArchiveMailRetry(t, ctx, target, oldID)
	account, err := authority.LookupPersonalByEmail(ctx, to)
	must(t, err)
	if account.EmailVerified {
		t.Fatal("temporary SMTP failure bypassed confirmation")
	}
	token := receive(ctx)
	waitLifecycleMail(t, ctx, target, 2)
	checkCharges(3)
	var accepted bool
	must(t, target.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM river_job WHERE args->>'payload_id'<>$1
		AND state='completed' AND attempt=2 AND metadata->>'oj_delivery_result'='accepted')`, oldID).Scan(&accepted))
	if !accepted || permit() == before {
		t.Fatal("fresh mail did not use the shared SMTP permit and record acceptance")
	}
	y := entryClient(t, b, ca)
	for _, attempt := range []struct {
		token string
		want  int
	}{{archived.Token().LinkValue(), http.StatusBadRequest}, {token, http.StatusNoContent}, {token, http.StatusBadRequest}} {
		body := `{"token":"` + attempt.token + `","new_password":"synthetic recovery fixture password"}`
		loginStatus(t, loginCall(y, activate, body, nil, nil), attempt.want)
	}
	for _, proc := range []*entryProcess{a, b} {
		proc.signal(t)
		proc.wait(t, true)
	}
}

func waitArchiveMailRetry(t *testing.T, ctx context.Context, pool *pgxpool.Pool, oldID string) {
	t.Helper()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		var pending bool
		must(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM river_job j
			JOIN identity_email_payloads p ON p.token_id=j.args->>'payload_id'
			WHERE p.token_id<>$1 AND j.state='available' AND j.attempt=1
			AND j.metadata->>'oj_delivery_result'='temporary_failure')`, oldID).Scan(&pending))
		if pending {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("temporary SMTP failure did not retain the encrypted payload for retry")
		case <-tick.C:
		}
	}
}

func configureArchiveMail(t *testing.T, ctx context.Context, f *fixture, c *sessionruntime.Config) (*pgxpool.Pool, *emailtoken.Protector, string) {
	t.Helper()
	const dbName = "fixture_archive_seals"
	_, err := f.pools[0].Exec(ctx, "CREATE DATABASE fixture_archive_seals")
	must(t, err)
	ledgerConfig := f.pools[0].Config()
	ledgerConfig.ConnConfig.Database = dbName
	ledger, err := pgxpool.NewWithConfig(ctx, ledgerConfig)
	must(t, err)
	t.Cleanup(ledger.Close)
	key := loginKey(t)
	keyFile := writeLifecycleJSON(t, "mail-keys.json", map[string]any{
		"format": "oj.email-payload-keys.v1", "current": "archive",
		"keys": map[string]string{"archive": base64.RawURLEncoding.EncodeToString(key[:])},
	})
	keys, err := emailtoken.LoadProtector(keyFile)
	must(t, err)
	must(t, mailbudget.Initialize(ctx, ledger))
	must(t, mailbudget.Provision(ctx, ledger, keys, 3))
	must(t, mailqueue.MigrateUp(ctx, f.pools[0]))
	must(t, mailqueue.ConfigureSendRate(ctx, f.pools[0], 100))
	budgetDB := c.Database
	budgetDB.Database, budgetDB.ApplicationName, budgetDB.Connections = dbName, "archive_mail_budget", 1
	c.EmailIntake = &sessionruntime.EmailIntakeConfig{
		Limits:         emailintake.Config{Capacity: 4, Workers: 1, MaxAge: 5 * time.Second},
		PayloadKeyFile: keyFile, BudgetDatabase: budgetDB,
	}
	bucket := protocol.LoginBucket{Count: 30, Window: time.Hour}
	c.EmailLimits = &protocol.EmailLimits{RequestGlobal: bucket, RequestSource: bucket, RequestMailbox: bucket,
		ConfirmGlobal: bucket, ConfirmSource: bucket, ConfirmToken: bucket}
	secrets, err := protocol.LoadLoginSecrets(c.SecretsFile)
	must(t, err)
	must(t, protocol.InitializeEmailLimits(ctx, f.store(t, c.Namespace, 0, protocol.OrdinaryPolicy()), secrets, *c.EmailLimits))
	weak := sha256.Sum256([]byte("synthetic blocked password"))
	c.EmailConfirmation, err = password.NewAcceptance(15, c.PasswordLimits.MaxPasswordBytes, [][32]byte{weak})
	must(t, err)
	blocklist := writeLifecycleJSON(t, "weak-passwords.json", map[string]any{"version": 1, "sha256": []string{hex.EncodeToString(weak[:])}})
	return ledger, keys, blocklist
}

func waitArchiveMailJob(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		var id string
		must(t, pool.QueryRow(ctx, `SELECT COALESCE((SELECT args->>'payload_id' FROM river_job LIMIT 1), '')`).Scan(&id))
		if id != "" {
			return id
		}
		select {
		case <-ctx.Done():
			t.Fatal("archived job was not queued")
		case <-tick.C:
		}
	}
}

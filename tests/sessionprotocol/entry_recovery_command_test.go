//go:build integration

package sessionprotocol_test

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func testRecoveryCommand(t *testing.T, f *fixture, api, command string, c sessionruntime.Config, ca *testpki.Authority,
	pool *pgxpool.Pool, owner, disabled identity.Personal, old protocol.Credential, pending *http.Cookie,
	checkReferenceQueue func(x, y *loginEndpoint, fresh protocol.Credential, epoch string)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	redisAddr, interruptedAddr := os.Getenv("OJ_PROTOCOL_REPLACEMENT_REDIS_ADDR"), os.Getenv("OJ_PROTOCOL_INTERRUPTED_REDIS_ADDR")
	if redisAddr != "redis-recovery:6379" || interruptedAddr != "redis-recovery-interrupted:6379" {
		t.Fatal("replacement Redis instances from the private protocol driver are required")
	}
	authority, err := identitypg.New(pool, f.hasher)
	must(t, err)
	const redisFailure = "replacement Redis unavailable, nonempty or only partly prepared; read status before retrying and do not reset this instance"
	run := func(path, expected string, success bool, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, command, append([]string{"session-recovery"}, args...)...)
		cmd.Env = []string{"OJ_RESTORED_SESSION_CONFIG_FILE=" + path}
		output, err := cmd.CombinedOutput()
		if (err == nil) != success || strings.TrimSpace(string(output)) != expected {
			t.Fatal("unexpected session recovery command result: " + args[0])
		}
	}
	receipt := func(state identity.RecoveryState) string {
		return "generation=" + state.Generation + " enabled=" + strconv.FormatBool(state.Enabled)
	}
	prior, err := authority.RecoveryState(ctx)
	must(t, err)
	closing := c
	closing.Redis.Addr = "127.0.0.1:1"
	closingPath := entryConfig(t, closing, ca, time.Second)
	run(closingPath, receipt(prior), true, "status", c.Database.Database)
	run(closingPath, sessionruntime.ErrConfig.Error(), false, "close", "wrong-database")
	run(closingPath, receipt(prior), true, "status", c.Database.Database)
	cmd := exec.CommandContext(ctx, command, "session-recovery", "close", c.Database.Database)
	cmd.Env = []string{"OJ_RESTORED_SESSION_CONFIG_FILE=" + closingPath}
	output, err := cmd.CombinedOutput()
	closed, stateErr := authority.RecoveryState(ctx)
	must(t, stateErr)
	if err != nil || strings.TrimSpace(string(output)) != receipt(closed) || closed.Enabled || !identity.ValidRecoveryID(closed.Generation) || closed.Generation == prior.Generation {
		t.Fatal("close did not rotate and keep the restored database closed with Redis unavailable")
	}
	oldPath := entryConfig(t, c, ca, time.Second)
	run(oldPath, sessionruntime.ErrRecoveryEpoch.Error(), false, "open", c.Database.Database, prior.Generation, "--accounts-reviewed")
	run(oldPath, redisFailure, false, "open", c.Database.Database, closed.Generation, "--accounts-reviewed")
	run(closingPath, receipt(closed), true, "status", c.Database.Database)
	if ttl, err := f.control.PTTL(ctx, tokenKey(c.Namespace, old)).Result(); err != nil || ttl <= 0 {
		t.Fatal("refusal changed the original Redis session")
	}
	// Reconcile a post-backup disable from the still-isolated source evidence.
	account, err := authority.State(ctx, disabled.ID)
	must(t, err)
	if !account.Enabled {
		t.Fatal("restored account must exhibit the archived state before reconciliation")
	}
	_, err = authority.DisableCAS(ctx, account.ID, account.Revision)
	must(t, err)
	blocklist := enableEntryPasswordChange(t, f, &c, changeLimits(), false)
	bucket := protocol.LoginBucket{Count: 10, Window: time.Hour}
	c.EmailLimits = &protocol.EmailLimits{RequestGlobal: bucket, RequestSource: bucket, RequestMailbox: bucket,
		ConfirmGlobal: bucket, ConfirmSource: bucket, ConfirmToken: bucket}
	interrupted := c
	interrupted.Redis.Addr = interruptedAddr
	interruptedPath := entryConfig(t, interrupted, ca, 3*time.Second, blocklist)
	// Force the real PostgreSQL update to fail after all Redis markers are written.
	_, err = pool.Exec(ctx, `CREATE FUNCTION reject_fixture_session_open() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'synthetic open failure'; END; $$;
		CREATE TRIGGER reject_fixture_session_open BEFORE UPDATE OF enabled ON identity_session_recovery
		FOR EACH ROW WHEN (NEW.enabled) EXECUTE FUNCTION reject_fixture_session_open()`)
	must(t, err)
	run(interruptedPath, "session recovery failed or outcome unknown; read status before retrying", false,
		"open", c.Database.Database, closed.Generation, "--accounts-reviewed")
	run(closingPath, receipt(closed), true, "status", c.Database.Database)
	client, err := protocol.NewClient(interrupted.Redis)
	must(t, err)
	t.Cleanup(func() { must(t, client.Close()) })
	count, err := client.DBSize(ctx).Result()
	must(t, err)
	if count != 3 {
		t.Fatal("failed open did not leave all three prepared markers for inspection")
	}
	_, err = pool.Exec(ctx, `DROP TRIGGER reject_fixture_session_open ON identity_session_recovery; DROP FUNCTION reject_fixture_session_open()`)
	must(t, err)
	run(interruptedPath, redisFailure, false, "open", c.Database.Database, closed.Generation, "--accounts-reviewed")
	run(closingPath, receipt(closed), true, "status", c.Database.Database)
	after, err := client.DBSize(ctx).Result()
	must(t, err)
	if after != count {
		t.Fatal("retry mutated the partly prepared Redis")
	}

	c.Redis.Addr = redisAddr
	path := entryConfig(t, c, ca, 3*time.Second, blocklist)
	run(path, "usage: migrate session-recovery status|close <database> or open <database> <generation> --accounts-reviewed", false,
		"open", c.Database.Database, closed.Generation)
	run(closingPath, receipt(closed), true, "status", c.Database.Database)
	opened := identity.RecoveryState{Generation: closed.Generation, Enabled: true}
	run(path, receipt(opened), true, "open", c.Database.Database, closed.Generation, "--accounts-reviewed")
	replacement, err := protocol.NewClient(c.Redis)
	must(t, err)
	t.Cleanup(func() { must(t, replacement.Close()) })
	if ttl, err := replacement.PTTL(ctx, "oj-email-limit:"+c.Namespace+":config").Result(); err != nil || ttl != -1 {
		t.Fatal("successful recovery did not leave a persistent email marker")
	}
	run(path, sessionruntime.ErrRecoveryEpoch.Error(), false, "open", c.Database.Database, closed.Generation, "--accounts-reviewed")
	var children []*entryProcess
	var endpoints []*loginEndpoint
	for _, name := range []string{"_recovery_a", "_recovery_b"} {
		replica := c
		replica.Database.ApplicationName += name
		child := entryStart(t, api, entryConfig(t, replica, ca, 3*time.Second, blocklist), "127.0.0.1:0")
		endpoint := entryClient(t, child, ca)
		children = append(children, child)
		endpoints = append(endpoints, endpoint)
	}
	if !time.Now().Before(pending.Expires) {
		t.Fatal("pending credential expired before the recovery check")
	}
	get := func(r *http.Request) { r.Method = http.MethodGet }
	for _, endpoint := range endpoints {
		loginStatus(t, loginCall(endpoint, "/api/v1/session", "", credentialCookies(old), get), 401)
		loginStatus(t, loginCall(endpoint, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil), 401)
		loginStatus(t, loginCall(endpoint, "/api/v1/session/login/begin", loginBodyFor(t, disabled), nil, nil), 401)
	}
	fresh := entryCredential(t, endpoints[0], owner, "synthetic protocol password")
	loginStatus(t, loginCall(endpoints[1], "/api/v1/session", "", credentialCookies(fresh), get), 200)
	checkReferenceQueue(endpoints[0], endpoints[1], fresh, opened.Generation)
	for _, child := range children {
		child.signal(t)
		child.wait(t, true)
	}
	t.Log("recovery commands reopened the archived database on a fresh Redis; reconciled disabled account stayed blocked and fresh login succeeded")
}

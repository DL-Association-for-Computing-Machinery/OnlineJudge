//go:build integration

package sessionprotocol_test

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func testEntryDatabaseRestore(t *testing.T, f *fixture, binary string) {
	command := entryProgram(t, "migrate")
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	config := runtimeConfig(t, f, namespace(t), true)
	owner, candidate := f.subject(t, false), f.subject(t, false)
	ca := testpki.New(t)
	start := func(c sessionruntime.Config) *entryProcess {
		return entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
	}
	stop := func(p *entryProcess) {
		t.Helper()
		p.signal(t)
		p.wait(t, true)
		if _, ok := p.event("API stopped gracefully"); !ok {
			t.Fatal("API did not confirm drain")
		}
	}
	read := func(e *loginEndpoint, c protocol.Credential, status int) {
		t.Helper()
		loginStatus(t, loginCall(e, "/api/v1/session", "", credentialCookies(c), func(r *http.Request) { r.Method = http.MethodGet }), status)
	}
	source := start(config)
	before := entryClient(t, source, ca)
	old := entryCredential(t, before, owner, "synthetic protocol password")
	pending := beginLogin(t, before, candidate)
	read(before, old, 200)
	saved, err := f.identities[0].RecoveryState(ctx)
	must(t, err)
	if !saved.Enabled {
		t.Fatal("snapshot must capture an open authority")
	}
	archive := filepath.Join(t.TempDir(), "postgres.dump")
	dumpEntryDatabase(t, ctx, config.Database, archive)

	// Advance the live source after the backup. The archive must restore the
	// earlier epoch, not silently read the source database after the rotation.
	retired, err := f.identities[0].BeginRecovery(ctx)
	must(t, err)
	read(before, old, 503)
	must(t, f.identities[0].FinishRecovery(ctx, retired))
	read(before, old, 401)
	stop(source)
	noRuntimeConnections(t, f, config.Database.ApplicationName)
	// This post-backup fact is known to the operator but missing from the archive.
	_, err = f.identities[0].DisableCAS(ctx, candidate.ID, candidate.Revision)
	must(t, err)

	config.Database = restoreEntryDatabase(t, ctx, config.Database, archive)
	target := config.Database
	pool, err := submissionpg.Open(ctx, "host="+target.Host+" port="+strconv.Itoa(int(target.Port))+" user=oj_protocol dbname=postgres sslmode=disable")
	must(t, err)
	t.Cleanup(pool.Close)
	authority, err := identitypg.New(pool, f.hasher)
	must(t, err)
	rolledBack, err := authority.RecoveryState(ctx)
	must(t, err)
	if rolledBack != saved || rolledBack.Generation == retired {
		t.Fatal("isolated restore did not roll back the source authority")
	}
	// Demonstrate actual revival at the storage boundary if recovery is skipped.
	// Do not start a restored API before closing and rotating its authority.
	probe, err := protocol.New(ctx, f.clients[0], authority, f.budget, config.Namespace, protocol.OrdinaryPolicy())
	must(t, err)
	view, err := probe.Read(ctx, old)
	must(t, err)
	if view.SubjectID != owner.ID || view.RecoveryID != saved.Generation {
		t.Fatal("old cookie did not match the restored authority before fencing")
	}
	fresh, err := authority.BeginRecovery(ctx)
	must(t, err)
	if fresh == saved.Generation || fresh == retired {
		t.Fatal("recovery reused an earlier authority")
	}
	wantError(t, authority.FinishRecovery(ctx, saved.Generation), identity.ErrConflict)
	closed := start(config)
	closed.wait(t, false)
	if _, ok := closed.event("API starting"); ok {
		t.Fatal("closed restore advertised an API listener")
	}
	must(t, authority.FinishRecovery(ctx, fresh))

	a := start(config)
	other := config
	other.Database.ApplicationName += "_b"
	b := start(other)
	x, y := entryClient(t, a, ca), entryClient(t, b, ca)
	ttl, err := f.control.PTTL(ctx, tokenKey(config.Namespace, old)).Result()
	must(t, err)
	if ttl <= 0 || !time.Now().Before(pending.Expires) {
		t.Fatal("old cookies were no longer live before the rejection checks")
	}
	for _, endpoint := range []*loginEndpoint{x, y} {
		read(endpoint, old, 401)
		loginStatus(t, loginCall(endpoint, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil), 401)
	}
	// Same restored account, Redis namespace and secret files: a new login must
	// succeed while the archived cookie still cannot be used.
	current := entryCredential(t, x, owner, "synthetic protocol password")
	read(y, current, 200)
	read(x, old, 401)
	pendingBeforeMaintenance := beginLogin(t, y, owner)
	stop(a)
	stop(b)
	t.Log("archive restored an old live cookie at storage boundary; closed target refused startup, both reopened APIs rejected it and accepted fresh login")
	t.Run("operator recovery on empty replacement Redis", func(t *testing.T) {
		testRecoveryCommand(t, f, binary, command, config, ca, pool, owner, candidate, current, pendingBeforeMaintenance)
	})
}

func dumpEntryDatabase(t *testing.T, ctx context.Context, source sessionruntime.DatabaseConfig, archive string) {
	t.Helper()
	if !strings.HasPrefix(source.Host, "/tmp/oj-protocol.") || source.Username != "oj_protocol" || source.Database != "postgres" {
		t.Fatal("restore test requires the driver's private source cluster")
	}
	entryRestoreCommand(t, ctx, "pg_dump", "--host", source.Host, "--port", strconv.Itoa(int(source.Port)),
		"--username", source.Username, "--dbname", source.Database, "--no-password", "--format=custom", "--file", archive)
}

func restoreEntryDatabase(t *testing.T, ctx context.Context, source sessionruntime.DatabaseConfig, archive string) sessionruntime.DatabaseConfig {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "oj-session-restore-")
	must(t, err)
	data := filepath.Join(root, "data")
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := os.Stat(filepath.Join(data, "postmaster.pid")); err == nil {
			entryRestoreCommand(t, cleanup, "pg_ctl", "-D", data, "-m", "fast", "-w", "stop")
		} else if !os.IsNotExist(err) {
			must(t, err)
		}
		must(t, os.RemoveAll(root))
	})
	entryRestoreCommand(t, ctx, "initdb", "-D", data, "--username=oj_protocol", "--auth-local=trust", "--auth-host=reject", "--no-locale", "--encoding=UTF8")
	entryRestoreCommand(t, ctx, "pg_ctl", "-D", data, "-l", filepath.Join(root, "server.log"), "-o", "-k "+root+" -p 5432 -c listen_addresses=''", "-w", "start")
	entryRestoreCommand(t, ctx, "pg_restore", "--host", root, "--port", "5432", "--username", "oj_protocol", "--dbname", "postgres",
		"--no-password", "--single-transaction", "--no-owner", "--no-privileges", archive)
	source.Host, source.Port = root, 5432
	return source
}

func entryRestoreCommand(t *testing.T, ctx context.Context, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LC_ALL=C"}
	if _, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("private PostgreSQL %s failed: %v", name, err)
	}
}

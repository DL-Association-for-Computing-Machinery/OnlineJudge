package sessionexperiment

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func privateRestoreConfig(t *testing.T) *pgx.ConnConfig {
	t.Helper()
	config, err := pgx.ParseConfig(os.Getenv("OJ_SESSION_DATABASE_URL"))
	if err != nil {
		t.Fatal("invalid private database fixture configuration")
	}
	// This destructive logical-restore experiment is confined to the driver's
	// disposable Unix-socket cluster, never an arbitrary caller-supplied database.
	if os.Getenv("OJ_SESSION_DATABASE_FIXTURE") != "1" ||
		!strings.HasPrefix(config.Host, "/tmp/oj-session-pg.") ||
		config.User != "oj_session" || config.Database != "postgres" || config.Password != "" {
		t.Fatal("requires driver's private PostgreSQL fixture")
	}
	return config
}

func validRestoreSchema(schema string) bool {
	return strings.HasPrefix(schema, "oj_restore_") && len(schema) <= 63 &&
		strings.IndexFunc(schema, func(c rune) bool {
			return c != '_' && (c < 'a' || c > 'z') && (c < '0' || c > '9')
		}) == -1
}

func restoreSchemaFixture(t *testing.T) (*fixture, *pgx.ConnConfig, string) {
	t.Helper()
	config := privateRestoreConfig(t)
	schema := "oj_restore_" + strings.ToLower(rand.Text())
	if !validRestoreSchema(schema) {
		t.Fatal("invalid generated fixture schema")
	}
	admin := newAuthority(t, "restore-admin")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Identifiers cannot be SQL parameters; only the internally generated,
	// allowlisted name is quoted with pgx. All row data remains parameterized.
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.pool.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		if _, err := admin.pool.Exec(cleanup, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("remove owned restore schema: %v", err)
		}
	})
	// The driver emits a keyword DSN. The schema is an internal safe identifier;
	// no DSN or credential is sent to subprocess logs or interpolated into a shell.
	t.Setenv("OJ_SESSION_DATABASE_URL", os.Getenv("OJ_SESSION_DATABASE_URL")+" search_path="+schema)
	f := setupAuth(t)
	return f, config, schema
}

func schemaSnapshotCommand(t *testing.T, config *pgx.ConnConfig, schema, archive string, restore bool) {
	t.Helper()
	if !validRestoreSchema(schema) {
		t.Fatal("restore target must be owned fixture schema")
	}
	name := "pg_dump"
	args := []string{"--host", config.Host, "--port", strconv.Itoa(int(config.Port)),
		"--username", config.User, "--dbname", config.Database, "--no-password",
		"--schema", schema, "--no-owner", "--no-privileges"}
	if restore {
		name = "pg_restore"
		args = append(args, "--clean", "--if-exists", "--single-transaction", archive)
	} else {
		args = append(args, "--format=custom", "--file", archive)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	// An empty PGSERVICE still selects a named service in libpq; remove inherited
	// PostgreSQL settings instead and provide every connection target explicitly.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "PG") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "PGPASSFILE=/dev/null", "PGCONNECT_TIMEOUT=3")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("private %s failed: %v: %s", name, err, output)
	}
}

func TestDatabaseRestoreRejectsRetiredRecoveryState(t *testing.T) {
	for _, scenario := range []string{"old-cookie", "late-finish"} {
		t.Run(scenario, func(t *testing.T) {
			f, config, schema := restoreSchemaFixture(t)
			ctx := context.Background()
			initial, err := f.stores[0].auth.current(ctx, "fixture-user")
			if err != nil {
				t.Fatal(err)
			}
			first := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			archive := filepath.Join(t.TempDir(), "auth.dump")
			schemaSnapshotCommand(t, config, schema, archive, false)
			middle := beginRecovery(t, f.stores[1].auth)
			finishRecovery(t, f.stores[1].auth, middle)
			retired := expect(t, f, 1, "POST", "/login-fixture", nil, 201).Cookies()[0]
			latest := beginRecovery(t, f.stores[1].auth)
			finishRecovery(t, f.stores[1].auth, latest)
			invalidate(t, f.stores[1].auth, true)
			last := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			for replica := range 2 {
				rejectWithoutCookie(t, f, replica, "GET", "/me", retired, 401)
				expect(t, f, replica, "GET", "/me", last, 200)
			}
			retained := memorySnapshot(t, f)
			if len(retained) != 6 {
				t.Fatal("three generations of session payloads/indexes not retained")
			}
			runID := redisRunID(t, f.stores[0].client)
			// External maintenance fencing: stop and drain every old API before
			// restore. Its backup also restores enabled=true; the DB gate alone
			// cannot safely coordinate the restore interval or another operator.
			for replica := range 2 {
				f.servers[replica].Close()
				f.stores[replica].auth.pool.Close()
			}
			schemaSnapshotCommand(t, config, schema, archive, true)
			authority := newAuthority(t, f.stores[0].auth.namespace)
			rolledBack, err := authority.current(ctx, "fixture-user")
			if err != nil || rolledBack != initial {
				t.Fatalf("database did not return to its backup state: %v", err)
			}
			if redisRunID(t, f.stores[0].client) != runID {
				t.Fatal("Redis restarted during database-only restore")
			}
			assertMemoryUnchanged(t, f, retained)
			current := beginRecovery(t, authority)
			resumed := setupAuthorizedReplicas(t, f.stores[0].prefix, 2,
				[]*authAuthority{newAuthority(t, authority.namespace), newAuthority(t, authority.namespace)})
			assertRecoveryClosed(t, resumed, retired)
			if scenario == "late-finish" {
				for _, old := range []authVersion{{recovery: middle}, {recovery: latest}} {
					if err := authority.finishRecovery(ctx, old.recovery); !errors.Is(err, errRecovering) {
						t.Fatalf("retired recovery completion reopened restored database: %v", err)
					}
				}
				assertRecoveryClosed(t, resumed, retired)
			}
			finishRecovery(t, authority, current)
			for replica := range 2 {
				for _, cookie := range []*http.Cookie{retired, first, last} {
					rejectWithoutCookie(t, resumed, replica, "GET", "/me", cookie, 401)
				}
			}
			if current == initial.recovery || current == middle || current == latest {
				t.Fatal("recovery identifier reused a previously issued generation")
			}
			assertThreeFreshSlots(t, resumed)
		})
	}
}

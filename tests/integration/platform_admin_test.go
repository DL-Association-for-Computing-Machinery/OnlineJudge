//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func TestPlatformAdminAuthority(t *testing.T) {
	f := newBacklogFixture(t)
	h := identityHasher(t, false)
	users, err := identitypg.New(f.a, h)
	must(t, err)
	roles, err := identitypg.NewPlatformAdmins(f.a)
	must(t, err)
	other, err := identitypg.NewPlatformAdmins(f.b)
	must(t, err)
	epoch, err := users.BeginRecovery(f.ctx)
	must(t, err)
	must(t, users.FinishRecovery(f.ctx, epoch))
	record := identityRecord(t, h, "synthetic platform administration fixture")
	makeAccount := func(id int, verified bool) identity.Personal {
		t.Helper()
		address, err := identity.ParseEmail(identityID(id) + "@example.invalid")
		must(t, err)
		p, err := users.InitializePersonal(f.ctx, identityID(id), address, true, record)
		must(t, err)
		if verified {
			_, err := f.a.Exec(f.ctx, `UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1`, p.ID)
			must(t, err)
		}
		return p
	}
	admin := makeAccount(901, true)
	unverified := makeAccount(902, false)
	change := identity.PlatformAdminChange{SubjectID: unverified.ID, Enabled: true, Operator: "fixture", Reason: "test qualification"}
	_, err = roles.Change(f.ctx, change)
	if !errors.Is(err, identity.ErrPlatformAdminRequired) {
		t.Fatalf("unverified account grant = %v", err)
	}
	actor := identity.PlatformAdminActor{SubjectID: admin.ID, SessionVersion: identity.SessionVersion{
		AuthGeneration: admin.AuthGeneration, RecoveryID: epoch,
	}, RoleRevision: 1}
	check := func(level pgx.TxIsoLevel, a identity.PlatformAdminActor, want error) {
		t.Helper()
		tx, err := f.a.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: level})
		must(t, err)
		defer func() { _ = tx.Rollback(f.ctx) }()
		if err := identitypg.LockPlatformAdmin(f.ctx, tx, a); !errors.Is(err, want) {
			t.Fatalf("platform admin guard = %v; want %v", err, want)
		}
	}
	check(pgx.ReadCommitted, actor, identity.ErrPlatformAdminRequired)
	change.SubjectID = admin.ID
	out := make(chan error, 2)
	go func() { _, err := roles.Change(f.ctx, change); out <- err }()
	go func() { _, err := other.Change(f.ctx, change); out <- err }()
	first, second := <-out, <-out
	if first != nil {
		first, second = second, first
	}
	if first != nil || !errors.Is(second, identity.ErrConflict) {
		t.Fatalf("concurrent grants = %v / %v", first, second)
	}
	check(pgx.RepeatableRead, actor, identity.ErrInvalid)
	tx, err := f.a.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	must(t, identitypg.LockPlatformAdmin(f.ctx, tx, actor))
	change.Enabled, change.ExpectedRevision = false, 1
	go func() { _, err := other.Change(f.ctx, change); out <- err }()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
wait:
	for {
		var blocked bool
		_, err := tx.Exec(f.ctx, `SELECT pg_stat_clear_snapshot()`)
		must(t, err)
		must(t, tx.QueryRow(f.ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE application_name=$1 AND pg_backend_pid() = ANY(pg_blocking_pids(pid)))`, f.appB).Scan(&blocked))
		if blocked {
			break wait
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("administrator revocation did not wait for the authorized transaction")
		}
	}
	must(t, tx.Commit(f.ctx))
	must(t, <-out)
	check(pgx.ReadCommitted, actor, identity.ErrPlatformAdminRequired)
	change.Enabled, change.ExpectedRevision = true, 2
	role, err := other.Change(f.ctx, change)
	must(t, err)
	check(pgx.ReadCommitted, actor, identity.ErrPlatformAdminRequired)
	actor.RoleRevision = 3
	if role.Revision != 3 || !role.Enabled {
		t.Fatal("regrant did not advance the role revision")
	}
	check(pgx.ReadCommitted, actor, nil)
	_, err = f.a.Exec(f.ctx, `UPDATE identity_personal_emails SET email_verified=false WHERE subject_id=$1`, admin.ID)
	must(t, err)
	check(pgx.ReadCommitted, actor, identity.ErrPlatformAdminRequired)
	_, err = f.a.Exec(f.ctx, `UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1`, admin.ID)
	must(t, err)
	nextEpoch, err := users.BeginRecovery(f.ctx)
	must(t, err)
	check(pgx.ReadCommitted, actor, identity.ErrRecovering)
	must(t, users.FinishRecovery(f.ctx, nextEpoch))
	check(pgx.ReadCommitted, actor, identity.ErrConflict)
	actor.RecoveryID = nextEpoch
	check(pgx.ReadCommitted, actor, nil)
	state, err := users.State(f.ctx, admin.ID)
	must(t, err)
	_, err = users.DisableCAS(f.ctx, admin.ID, state.Revision)
	must(t, err)
	check(pgx.ReadCommitted, actor, identity.ErrConflict)
	var history int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM identity_platform_admin_history
		WHERE subject_id=$1 AND maintenance_operator=$2 AND reason=$3`, admin.ID, change.Operator, change.Reason).Scan(&history))
	if history != 3 {
		t.Fatal("role history did not retain the three audited revisions")
	}
	rows, err := f.a.Query(f.ctx, `EXPLAIN (COSTS OFF) SELECT subject_id, revision, enabled
		FROM identity_platform_admin_history WHERE subject_id=$1 ORDER BY revision DESC LIMIT 1`, admin.ID)
	must(t, err)
	var plan string
	for rows.Next() {
		var line string
		must(t, rows.Scan(&line))
		plan += line + "\n"
	}
	must(t, rows.Err())
	rows.Close()
	if !strings.Contains(plan, "identity_platform_admin_history_pkey") {
		t.Fatalf("latest role lookup did not use the primary key: %s", plan)
	}
	t.Log(plan)
	if _, err := f.provider.Down(f.ctx); err == nil {
		t.Fatal("downgrade discarded platform administrator history")
	}
	must(t, db.CheckSchema(f.ctx, f.a))
}

func TestEmptyPlatformAdminMigrationRollback(t *testing.T) {
	f := newBacklogFixture(t)
	_, err := f.provider.Down(f.ctx)
	must(t, err)
	if db.CheckSchema(f.ctx, f.a) == nil {
		t.Fatal("new binary accepted the downgraded schema")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
}

func TestPlatformAdminMaintenanceCommand(t *testing.T) {
	f := newBacklogFixture(t)
	h := identityHasher(t, false)
	users, err := identitypg.New(f.a, h)
	must(t, err)
	id := identityID(903)
	address, err := identity.ParseEmail(id + "@example.invalid")
	must(t, err)
	_, err = users.InitializePersonal(f.ctx, id, address, true, identityRecord(t, h, "synthetic maintenance fixture"))
	must(t, err)
	_, err = f.a.Exec(f.ctx, `UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1`, id)
	must(t, err)
	binary, err := filepath.Abs("../../.local/bin/migrate")
	must(t, err)
	var schema string
	must(t, f.a.QueryRow(f.ctx, `SELECT current_schema()`).Scan(&schema))
	cfg := f.a.Config().ConnConfig
	u := url.URL{Scheme: "postgres", User: url.User(cfg.User), Host: "localhost", Path: "/" + cfg.Database}
	u.RawQuery = url.Values{"host": {cfg.Host}, "sslmode": {"disable"}, "search_path": {schema}}.Encode()
	run := func(want identity.PlatformAdminRole, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(f.ctx, binary, append([]string{"platform-admin"}, args...)...)
		cmd.Env = append(os.Environ(), "OJ_DATABASE_URL="+u.String())
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		err := cmd.Run()
		if want.Revision == 0 {
			if err == nil || out.Len() != 0 || stderr.Len() == 0 {
				t.Fatal("maintenance failure did not produce a diagnostic without a success response")
			}
			return
		}
		must(t, err)
		var role identity.PlatformAdminRole
		must(t, json.Unmarshal(out.Bytes(), &role))
		if role != want {
			t.Fatalf("platform administrator role = %+v; want %+v", role, want)
		}
	}
	inspect := []string{"inspect", "--subject", id}
	set := []string{"set", "--subject", id, "--expected-revision", "0", "--enabled", "true", "--operator", "fixture", "--reason", "command acceptance"}
	grant := identity.PlatformAdminRole{SubjectID: id, Revision: 1, Enabled: true}
	run(identity.PlatformAdminRole{}, inspect...)
	run(grant, set...)
	run(identity.PlatformAdminRole{}, set...)
	run(grant, inspect...)
	set[4], set[6] = "1", "false"
	run(identity.PlatformAdminRole{SubjectID: id, Revision: 2}, set...)
}

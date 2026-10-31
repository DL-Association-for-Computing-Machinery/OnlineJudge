package sessionexperiment

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func postgresRestoreState(t *testing.T, s *postgresSessionStore) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var b []byte
	err := s.pool.QueryRow(ctx, `SELECT jsonb_build_object(
		'gate',(SELECT to_jsonb(r) FROM session_pg_recovery_fixture r WHERE namespace=$1),
		'accounts',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY subject),'[]'::jsonb)
			FROM session_pg_account_fixture a WHERE namespace=$1),
		'subjects',(SELECT coalesce(jsonb_agg(to_jsonb(p) ORDER BY subject),'[]'::jsonb)
			FROM session_pg_subject_fixture p WHERE namespace=$1),
		'sessions',(SELECT coalesce(jsonb_agg(to_jsonb(s) ORDER BY token),'[]'::jsonb)
			FROM session_pg_store_fixture s WHERE namespace=$1))`, s.namespace).Scan(&b)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func assertRestoredPayloadsCleared(t *testing.T, s *postgresSessionStore, backup []byte, generation string) {
	t.Helper()
	var expected, actual map[string]interface{}
	if err := json.Unmarshal(backup, &expected); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(postgresRestoreState(t, s), &actual); err != nil {
		t.Fatal(err)
	}
	gate := expected["gate"].(map[string]interface{})
	gate["generation"], gate["enabled"] = generation, false
	for _, row := range expected["sessions"].([]interface{}) {
		row.(map[string]interface{})["data"] = nil
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Fatal("recovery changed restored fields beyond gate and payload invalidation")
	}
}

func restoredPostgresStore(t *testing.T, namespace string) *postgresSessionStore {
	t.Helper()
	return &postgresSessionStore{
		pool: newAuthority(t, namespace).pool, namespace: namespace, account: true, recovery: true,
	}
}

func TestPostgresSessionsAndAccountsRestore(t *testing.T) {
	config, schema := privateRestoreSchema(t)
	f := setupPostgresSessionOptions(t, true, true)
	s := f.stores[0]
	ctx := context.Background()
	initial, open := postgresRecoveryState(t, s)
	if !open {
		t.Fatal("backup gate must initially be open")
	}
	first := expect(t, f.api, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	for i := range 2 {
		expect(t, f.api, i, "GET", "/me", first, 200)
	}
	original, found, err := s.FindCtx(ctx, first.Value)
	if err != nil || !found {
		t.Fatal("backup session is not readable", err)
	}
	backupState := postgresRestoreState(t, s)
	backupAccounts := postgresAccountsSnapshot(t, s)

	// Keep a changed-after-backup sentinel outside the archived schema. A restore
	// must preserve its newer value, not merely an unchanged pre-backup value.
	outside := "oj_restore_" + strings.ToLower(rand.Text())
	if !validRestoreSchema(outside) {
		t.Fatal("invalid sentinel schema")
	}
	admin := newAuthority(t, "pg-restore-sentinel")
	outsideID, table := pgx.Identifier{outside}.Sanitize(), pgx.Identifier{outside, "sentinel"}.Sanitize()
	if _, err := admin.pool.Exec(ctx, "CREATE SCHEMA "+outsideID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := admin.pool.Exec(cleanup, "DROP SCHEMA "+outsideID+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	if _, err := admin.pool.Exec(ctx, "CREATE TABLE "+table+" (marker integer NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.pool.Exec(ctx, "INSERT INTO "+table+" VALUES($1)", 1); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "postgres-sessions.dump")
	schemaSnapshotCommand(t, config, schema, archive, false)
	if _, err := admin.pool.Exec(ctx, "UPDATE "+table+" SET marker=$1", 2); err != nil {
		t.Fatal(err)
	}

	afterBackup := expect(t, f.api, 1, "POST", "/login-fixture", nil, 201).Cookies()[0]
	for i := range 2 {
		expect(t, f.api, i, "GET", "/me", afterBackup, 200)
	}
	if err := s.invalidateAccount(ctx, "fixture-user", false); err != nil {
		t.Fatal(err)
	}
	retired, err := s.beginRecovery(ctx)
	if err != nil || retired == initial {
		t.Fatal("recovery before restore did not rotate", err)
	}
	if err := s.finishRecovery(ctx, retired); err != nil {
		t.Fatal(err)
	}
	if generation, enabled := postgresAccountState(t, s); generation != 2 || enabled || postgresRows(t, s, true) != 0 {
		t.Fatal("post-backup revocation was not persisted")
	}
	for i := range 2 {
		for _, cookie := range []*http.Cookie{first, afterBackup} {
			rejectWithoutCookie(t, f.api, i, "GET", "/me", cookie, 401)
		}
		rejectWithoutCookie(t, f.api, i, "POST", "/login-fixture", nil, 401)
	}

	// The backup contains enabled=true. Only this external stop/drain boundary
	// fences the restore interval; a gate restored by the same backup cannot.
	for _, server := range f.api.servers {
		server.Close()
	}
	for _, store := range f.stores {
		store.pool.Close()
	}
	schemaSnapshotCommand(t, config, schema, archive, true)
	observer := restoredPostgresStore(t, s.namespace)
	if !bytes.Equal(backupState, postgresRestoreState(t, observer)) {
		t.Fatal("four tables did not restore exactly to their backup state")
	}
	var sentinel int
	if err := admin.pool.QueryRow(ctx, "SELECT marker FROM "+table).Scan(&sentinel); err != nil || sentinel != 2 {
		t.Fatal("targeted restore changed data outside the owned schema", err)
	}
	// Controlled negative proof while every API is stopped: simply reopening
	// this restored database would revive a previously revoked credential.
	revived, found, err := observer.FindCtx(ctx, first.Value)
	if err != nil || !found || !bytes.Equal(original, revived) {
		t.Fatal("negative restore probe did not find the original credential", err)
	}
	if _, found, err := observer.FindCtx(ctx, afterBackup.Value); err != nil || found {
		t.Fatal("backup unexpectedly retained a later credential", err)
	}
	current, err := observer.beginRecovery(ctx)
	if err != nil || current == initial || current == retired {
		t.Fatal("restored database did not get a fresh recovery identity", err)
	}
	assertRestoredPayloadsCleared(t, observer, backupState, current)
	closedState := postgresRestoreState(t, observer)
	for _, old := range []string{initial, retired} {
		if err := observer.finishRecovery(ctx, old); !errors.Is(err, errRecovering) || !bytes.Equal(closedState, postgresRestoreState(t, observer)) {
			t.Fatal("old recovery completion changed the restored gate", err)
		}
	}
	observer.pool.Close()
	resumed := servePostgresSessions(t, [2]*postgresSessionStore{
		restoredPostgresStore(t, s.namespace), restoredPostgresStore(t, s.namespace),
	})
	for i := range 2 {
		for _, path := range []string{"/me", "/login-fixture", "/activity-fixture", "/logout"} {
			method, cookie := "POST", first
			if path == "/me" {
				method = "GET"
			}
			if path == "/login-fixture" {
				cookie = nil
			}
			rejectWithoutCookie(t, resumed.api, i, method, path, cookie, 503)
		}
	}
	if !bytes.Equal(closedState, postgresRestoreState(t, resumed.stores[0])) {
		t.Fatal("closed HTTP requests modified restored state")
	}
	if err := resumed.stores[1].finishRecovery(ctx, current); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		for _, cookie := range []*http.Cookie{first, afterBackup} {
			rejectWithoutCookie(t, resumed.api, i, "GET", "/me", cookie, 401)
		}
	}
	// This is the snapshot's enabled synthetic subject, not a claim that the
	// newer account disable was recovered or that real login may be reopened.
	for i := range 3 {
		cookie := expect(t, resumed.api, i%2, "POST", "/login-fixture", nil, 201).Cookies()[0]
		for replica := range 2 {
			expect(t, resumed.api, replica, "GET", "/me", cookie, 200)
		}
	}
	rejectWithoutCookie(t, resumed.api, 1, "POST", "/login-fixture", nil, 409)
	if !bytes.Equal(backupAccounts, postgresAccountsSnapshot(t, resumed.stores[0])) || postgresRows(t, resumed.stores[0], true) != 3 {
		t.Fatal("recovery changed restored account facts or admission quota")
	}
}

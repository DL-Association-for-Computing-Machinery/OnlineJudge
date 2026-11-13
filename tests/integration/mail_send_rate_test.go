//go:build integration

package integration_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
)

func TestConfigureAccountMailSendRateCommand(t *testing.T) {
	f := newBacklogFixture(t)
	path, err := filepath.Abs("../../.local/bin/migrate")
	must(t, err)
	var schema string
	must(t, f.a.QueryRow(f.ctx, "SELECT current_schema()").Scan(&schema))
	dsn := f.a.Config().ConnString() + " search_path=" + schema
	configure := func(value string, wantOK bool) {
		t.Helper()
		cmd := exec.CommandContext(f.ctx, path, "mail-send-rate", value)
		cmd.Env = []string{"OJ_DATABASE_URL=" + dsn}
		output, err := cmd.CombinedOutput()
		if (err == nil) != wantOK {
			t.Fatalf("unexpected maintenance command result: %s", output)
		}
	}
	configure("99", false)
	configure("60001", false)
	configure("30000", true)
	gate, err := mailqueue.NewSendRate(f.ctx, f.b)
	must(t, err)
	allowed, err := gate.TryTake(f.ctx)
	must(t, err)
	if !allowed {
		t.Fatal("configured gate rejected first permit")
	}
	read := func() (int32, time.Time) {
		t.Helper()
		var spacing int32
		var next time.Time
		must(t, f.b.QueryRow(f.ctx, "SELECT spacing_ms, next_permit_at FROM account_mail_send_rate").Scan(&spacing, &next))
		return spacing, next
	}
	_, cutoff := read()
	configure("30000", true)
	if spacing, next := read(); spacing != 30000 || !next.Equal(cutoff) {
		t.Fatal("reapplying policy reset the permit deadline")
	}
	configure("10000", true)
	if spacing, next := read(); spacing != 10000 || !next.Equal(cutoff) {
		t.Fatal("shorter spacing released the existing permit early")
	}
	configure("20000", true)
	if spacing, next := read(); spacing != 20000 || !next.Equal(cutoff.Add(10*time.Second)) {
		t.Fatal("longer spacing did not preserve the in-flight cooldown")
	}
	// Hold a permit update open, as if the command arrived before the
	// sender committed. The upsert must apply to the just-committed cutoff.
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	var pendingCutoff time.Time
	must(t, tx.QueryRow(f.ctx, `UPDATE account_mail_send_rate
		SET next_permit_at=clock_timestamp()+interval '20 seconds'
		RETURNING next_permit_at`).Scan(&pendingCutoff))
	cmd := exec.CommandContext(f.ctx, path, "mail-send-rate", "40000")
	cmd.Env = []string{"OJ_DATABASE_URL=" + dsn + " application_name=mail_rate_test_maintenance"}
	must(t, cmd.Start())
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	waitForRateCommandLock(t, f)
	must(t, tx.Commit(f.ctx))
	must(t, <-waited)
	if spacing, next := read(); spacing != 40000 || !next.Equal(pendingCutoff.Add(20*time.Second)) {
		t.Fatal("policy edit did not use the concurrent sender's committed cutoff")
	}
	allowed, err = gate.TryTake(f.ctx)
	must(t, err)
	if allowed {
		t.Fatal("policy edit opened another permit")
	}
}

func waitForRateCommandLock(t *testing.T, f backlogFixture) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		must(t, f.b.QueryRow(f.ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock'
		)`, "mail_rate_test_maintenance").Scan(&waiting))
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("maintenance command never waited for the sender row lock")
}

func TestAccountMailSendRateAcrossPools(t *testing.T) {
	f := newBacklogFixture(t)
	must(t, db.CheckSchema(f.ctx, f.a))
	if _, err := mailqueue.NewSendRate(f.ctx, f.a); err == nil {
		t.Fatal("missing operator configuration enabled sends")
	}
	_, err := f.a.Exec(f.ctx, "INSERT INTO account_mail_send_rate (spacing_ms) VALUES ($1)", 30000)
	must(t, err)
	first, err := mailqueue.NewSendRate(f.ctx, f.a)
	must(t, err)
	second, err := mailqueue.NewSendRate(f.ctx, f.b)
	must(t, err)
	concurrent := func() {
		t.Helper()
		start := make(chan struct{})
		var admitted atomic.Int32
		var workers sync.WaitGroup
		for i := range 12 {
			workers.Go(func() {
				<-start
				rate := first
				if i%2 == 1 {
					rate = second
				}
				ok, err := rate.TryTake(f.ctx)
				if err != nil {
					t.Error("concurrent reservation failed")
				}
				if ok {
					admitted.Add(1)
				}
			})
		}
		close(start)
		workers.Wait()
		if admitted.Load() != 1 {
			t.Fatalf("expected one shared permit, got %d", admitted.Load())
		}
	}
	concurrent()
	var spacingSet bool
	must(t, f.a.QueryRow(f.ctx, "SELECT next_permit_at > clock_timestamp() + interval '20 seconds' FROM account_mail_send_rate").Scan(&spacingSet))
	if !spacingSet {
		t.Fatal("permit did not advance the shared cutoff")
	}
	reopened, err := mailqueue.NewSendRate(f.ctx, f.b)
	must(t, err)
	if ok, err := reopened.TryTake(f.ctx); err != nil || ok {
		t.Fatal("creating a new adapter reset the cutoff")
	}
	// Move the stored clock boundary rather than wait in real time. Idle
	// periods do not accumulate a backlog of unused permits.
	_, err = f.a.Exec(f.ctx, "UPDATE account_mail_send_rate SET next_permit_at = clock_timestamp() - interval '1 hour'")
	must(t, err)
	concurrent()

	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, "SELECT singleton FROM account_mail_send_rate FOR UPDATE")
	must(t, err)
	canceled, cancel := context.WithTimeout(f.ctx, 100*time.Millisecond)
	defer cancel()
	if ok, err := second.TryTake(canceled); err == nil || ok || canceled.Err() == nil {
		t.Fatal("reservation waiting for the row lock did not fail closed")
	}
	must(t, tx.Rollback(f.ctx))
	_, err = f.provider.Down(f.ctx)
	must(t, err)
	var version int64
	must(t, f.a.QueryRow(f.ctx, "SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1").Scan(&version))
	if version != 21 {
		t.Fatalf("rollback schema = %d, want 21", version)
	}
	if ok, err := second.TryTake(f.ctx); err == nil || ok {
		t.Fatal("missing schema allowed a send")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	if _, err := mailqueue.NewSendRate(f.ctx, f.a); err == nil {
		t.Fatal("recreated schema automatically restored a send rate")
	}
}

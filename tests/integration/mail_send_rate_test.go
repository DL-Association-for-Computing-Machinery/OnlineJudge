//go:build integration

package integration_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
)

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

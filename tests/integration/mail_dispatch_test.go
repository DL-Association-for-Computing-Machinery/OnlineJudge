//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
)

func TestMailDispatchFencesRescuedClaim(t *testing.T) {
	f := newBacklogFixture(t)
	must(t, mailqueue.MigrateUp(f.ctx, f.a))
	queue, err := mailqueue.NewEnqueuer(f.ctx, f.a)
	must(t, err)
	h := identityHasher(t, false)
	issuer, err := identitypg.New(f.a, h)
	must(t, err)
	email, err := identity.ParseEmail("dispatcher@example.invalid")
	must(t, err)
	subject := identityID(1)
	record := identityRecord(t, h, "synthetic dispatch test password")
	_, err = issuer.InitializePersonal(f.ctx, subject, email, true, record)
	must(t, err)
	epoch, err := issuer.BeginRecovery(f.ctx)
	must(t, err)
	must(t, issuer.FinishRecovery(f.ctx, epoch))
	protector, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	id, err := issuer.IssueEmailToken(f.ctx, subject, identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch},
		emailtoken.Activation, protector, func(context.Context, string) error { return nil }, queue.Enqueue)
	must(t, err)
	a, err := mailqueue.NewDispatcher(f.ctx, f.a)
	must(t, err)
	b, err := mailqueue.NewDispatcher(f.ctx, f.b)
	must(t, err)
	check := func(wantState string, wantAttempt int, wantResult string, wantPayload bool) {
		t.Helper()
		var state, result string
		var attempt int
		var payload bool
		must(t, f.a.QueryRow(f.ctx, `SELECT state::text, attempt, COALESCE(metadata->>'oj_delivery_result',''),
			EXISTS(SELECT 1 FROM identity_email_payloads WHERE token_id=$1)
			FROM river_job WHERE args->>'payload_id'=$1`, id).Scan(&state, &attempt, &result, &payload))
		if state != wantState || attempt != wantAttempt || result != wantResult || payload != wantPayload {
			t.Fatalf("state/attempt/result/payload = %s/%d/%s/%t", state, attempt, result, payload)
		}
	}
	run := func(d *mailqueue.Dispatcher, outcome mailqueue.DeliveryResult) {
		t.Helper()
		claimed, err := d.RunOne(f.ctx, func(_ context.Context, payload string) mailqueue.DeliveryResult {
			if payload != id {
				t.Error("wrong payload dispatched")
			}
			return outcome
		})
		must(t, err)
		if !claimed {
			t.Fatal("expected a claim")
		}
	}
	makeDue := func() {
		t.Helper()
		_, err := f.a.Exec(f.ctx, "UPDATE river_job SET scheduled_at=clock_timestamp() WHERE args->>'payload_id'=$1", id)
		must(t, err)
	}
	run(a, mailqueue.DeliveryDeferred)
	check("available", 0, "deferred", true)
	makeDue()
	run(a, mailqueue.DeliveryUnknown)
	check("available", 1, "unknown", true)
	makeDue()

	oldStarted, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	oldResult := make(chan error, 1)
	go func() {
		_, err := a.RunOne(f.ctx, func(ctx context.Context, _ string) mailqueue.DeliveryResult {
			close(oldStarted)
			select {
			case <-release:
			case <-ctx.Done():
			}
			return mailqueue.DeliveryAccepted
		})
		oldResult <- err
	}()
	select {
	case <-oldStarted:
	case err := <-oldResult:
		t.Fatalf("old claim failed to start: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("old claim did not start")
	}
	rescued, err := b.RescueOne(f.ctx)
	must(t, err)
	if rescued {
		t.Fatal("rescued a fresh claim")
	}
	var oldClaimTime time.Time
	must(t, f.a.QueryRow(f.ctx, "SELECT attempted_at FROM river_job WHERE args->>'payload_id'=$1", id).Scan(&oldClaimTime))
	_, err = f.a.Exec(f.ctx, "UPDATE river_job SET attempted_at=clock_timestamp()-interval '3 minutes' WHERE args->>'payload_id'=$1", id)
	must(t, err)
	rescued, err = b.RescueOne(f.ctx)
	must(t, err)
	if !rescued {
		t.Fatal("abandoned claim was not rescued")
	}
	check("available", 2, "abandoned", true)
	makeDue()
	claimed, err := b.RunOne(f.ctx, func(context.Context, string) mailqueue.DeliveryResult {
		var newClaimTime time.Time
		must(t, f.a.QueryRow(f.ctx, "SELECT attempted_at FROM river_job WHERE args->>'payload_id'=$1", id).Scan(&newClaimTime))
		// Equalize timestamps so the test also requires a fence other
		// than the artificial age used to trigger rescue.
		_, err := f.a.Exec(f.ctx, "UPDATE river_job SET attempted_at=$2 WHERE args->>'payload_id'=$1", id, oldClaimTime)
		must(t, err)
		close(release)
		select {
		case err := <-oldResult:
			if !errors.Is(err, mailqueue.ErrStaleClaim) {
				t.Errorf("old claimant returned %v, want stale claim", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("old claimant did not finish")
		}
		check("running", 3, "abandoned", true)
		// Restore this claim's true timestamp before its own completion.
		_, err = f.a.Exec(f.ctx, "UPDATE river_job SET attempted_at=$2 WHERE args->>'payload_id'=$1", id, newClaimTime)
		must(t, err)
		return mailqueue.DeliveryAccepted
	})
	must(t, err)
	if !claimed {
		t.Fatal("rescued job was not reclaimed")
	}
	check("completed", 3, "accepted", false)

	subject = identityID(2)
	secondEmail, err := identity.ParseEmail("dispatcher-retry@example.invalid")
	must(t, err)
	_, err = issuer.InitializePersonal(f.ctx, subject, secondEmail, true, record)
	must(t, err)
	id, err = issuer.IssueEmailToken(f.ctx, subject, identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch},
		emailtoken.Activation, protector, func(context.Context, string) error { return nil }, queue.Enqueue)
	must(t, err)
	for i := 1; i <= mailqueue.MaxAttempts; i++ {
		run(b, mailqueue.DeliveryRetry)
		if i < mailqueue.MaxAttempts {
			check("available", i, "temporary_failure", true)
			makeDue()
		}
	}
	check("discarded", mailqueue.MaxAttempts, "temporary_failure", false)
	claimed, err = b.RunOne(f.ctx, func(context.Context, string) mailqueue.DeliveryResult {
		t.Error("discarded job was sent again")
		return mailqueue.DeliveryAccepted
	})
	must(t, err)
	if claimed {
		t.Fatal("discarded job was reclaimed")
	}
}

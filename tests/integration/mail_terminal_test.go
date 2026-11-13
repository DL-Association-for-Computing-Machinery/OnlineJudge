//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
)

type terminalWorkerFixture struct {
	river.WorkerDefaults[mailqueue.Args]
	pool *pgxpool.Pool
	run  func(context.Context, pgx.Tx, *river.Job[mailqueue.Args]) error
}

func (w *terminalWorkerFixture) Work(ctx context.Context, job *river.Job[mailqueue.Args]) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := w.run(ctx, tx, job); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func TestMailTerminalWithRiverClaim(t *testing.T) {
	f := newBacklogFixture(t)
	must(t, mailqueue.MigrateUp(f.ctx, f.a))
	queue, err := mailqueue.NewEnqueuer(f.ctx, f.a)
	must(t, err)
	h := identityHasher(t, false)
	issuer, err := identitypg.New(f.a, h)
	must(t, err)
	email, err := identity.ParseEmail("terminal@example.invalid")
	must(t, err)
	subject := identityID(1)
	record := identityRecord(t, h, "synthetic terminal fixture password")
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
	reader, err := identitypg.NewEmailDeliveryReader(f.a, protector)
	must(t, err)
	prepared, err := reader.Prepare(f.ctx, id)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `CREATE FUNCTION fail_fixture_retirement() RETURNS trigger
		LANGUAGE plpgsql AS $$
		BEGIN
			IF current_setting('oj.fixture_fail_retirement', true) = 'on' THEN
				RAISE EXCEPTION 'synthetic retirement failure';
			END IF;
			RETURN OLD;
		END;
		$$;
		CREATE TRIGGER fixture_retirement BEFORE DELETE ON identity_email_payloads
			FOR EACH ROW EXECUTE FUNCTION fail_fixture_retirement();`)
	must(t, err)
	type observation struct {
		state   string
		payload bool
	}
	observe := func(ctx context.Context) (o observation, err error) {
		err = f.a.QueryRow(ctx, `SELECT j.state::text,
			EXISTS(SELECT 1 FROM identity_email_payloads WHERE token_id=$1)
			FROM river_job j WHERE j.args->>'payload_id'=$1`, id).Scan(&o.state, &o.payload)
		return
	}
	result := make(chan error, 1)
	worker := &terminalWorkerFixture{pool: f.b, run: func(ctx context.Context, tx pgx.Tx, job *river.Job[mailqueue.Args]) (retErr error) {
		defer func() { result <- retErr }()
		old := *job
		if err := mailqueue.CompleteTx(context.Background(), tx, job); !errors.Is(err, identity.ErrInvalid) {
			return errors.New("non-worker context was not rejected")
		}
		row := *job.JobRow
		row.AttemptedAt = new(job.AttemptedAt.Add(-time.Second))
		old.JobRow = &row
		if err := mailqueue.CompleteTx(ctx, tx, &old); !errors.Is(err, mailqueue.ErrStaleClaim) {
			return errors.New("older claim was not rejected")
		}
		old.Args.PayloadID = identityID(99)
		row.AttemptedAt = job.AttemptedAt
		if err := mailqueue.CompleteTx(ctx, tx, &old); !errors.Is(err, mailqueue.ErrStaleClaim) {
			return errors.New("different payload was not rejected")
		}
		old.Args = job.Args
		row.Attempt++
		if err := mailqueue.CompleteTx(ctx, tx, &old); !errors.Is(err, mailqueue.ErrStaleClaim) {
			return errors.New("different attempt was not rejected")
		}
		row.Attempt = job.Attempt
		row.AttemptedBy = []string{"different-executor"}
		if err := mailqueue.CompleteTx(ctx, tx, &old); !errors.Is(err, mailqueue.ErrStaleClaim) {
			return errors.New("different executor was not rejected")
		}
		checkRunning := func() error {
			got, err := observe(ctx)
			if err != nil || got != (observation{"running", true}) {
				return errors.New("completion or retirement visible before commit")
			}
			return nil
		}
		savepoint, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := savepoint.Exec(ctx, `UPDATE river_job SET metadata=metadata || '{"cancel_attempted_at":true}' WHERE id=$1`, job.ID); err != nil {
			return err
		}
		if err := mailqueue.CompleteTx(ctx, savepoint, job); !errors.Is(err, mailqueue.ErrStaleClaim) {
			return errors.New("revoked claim was not rejected")
		}
		if err := savepoint.Rollback(ctx); err != nil {
			return err
		}
		savepoint, err = tx.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := savepoint.Exec(ctx, "SET LOCAL oj.fixture_fail_retirement = 'on'"); err != nil {
			return err
		}
		if err := mailqueue.CompleteTx(ctx, savepoint, job); !errors.Is(err, identity.ErrStore) {
			return errors.New("delete failure did not reject completion")
		}
		if err := savepoint.Rollback(ctx); err != nil {
			return err
		}
		if err := checkRunning(); err != nil {
			return err
		}
		savepoint, err = tx.Begin(ctx)
		if err != nil {
			return err
		}
		if err := mailqueue.CompleteTx(ctx, savepoint, job); err != nil {
			return err
		}
		if err := checkRunning(); err != nil {
			return err
		}
		if err := savepoint.Rollback(ctx); err != nil {
			return err
		}
		if err := checkRunning(); err != nil {
			return err
		}
		return mailqueue.CompleteTx(ctx, tx, job)
	}}
	workers := river.NewWorkers()
	must(t, river.AddWorkerSafely(workers, worker))
	client, err := river.NewClient(riverpgxv5.New(f.b), &river.Config{
		Queues: map[string]river.QueueConfig{mailqueue.Queue: {MaxWorkers: 1}}, Workers: workers, PollOnly: true,
	})
	must(t, err)
	events, unsubscribe := client.Subscribe(river.EventKindJobCompleted, river.EventKindJobFailed)
	defer unsubscribe()
	must(t, client.Start(f.ctx))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		must(t, client.Stop(ctx))
	})
	select {
	case err := <-result:
		must(t, err)
	case <-f.ctx.Done():
		t.Fatal("River never claimed the committed mail job")
	}
	select {
	case event := <-events:
		if event.Kind != river.EventKindJobCompleted || event.Job.State != rivertype.JobStateCompleted {
			t.Fatal("River did not confirm the completed job")
		}
	case <-f.ctx.Done():
		t.Fatal("River did not report completion")
	}
	got, err := observe(f.ctx)
	must(t, err)
	if got != (observation{"completed", false}) {
		t.Fatal("final job and encrypted payload diverged")
	}
	must(t, issuer.ActivateEmail(f.ctx, prepared.Token(), record))
}

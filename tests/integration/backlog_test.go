//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func TestBacklogAdmission(t *testing.T) {
	t.Run("policy absent invalid and historical receipts", func(t *testing.T) {
		f := newBacklogFixture(t)
		_, _, err := f.first.Accept(f.ctx, "user", "training", "backlog-no-policy-01", f.request)
		if !errors.Is(err, submission.ErrAdmissionPolicy) {
			t.Fatalf("missing policy: %v", err)
		}
		for _, limits := range []postgres.BacklogLimits{{}, {Global: 1, Subject: 2}, {Global: -1, Subject: 1}} {
			if !errors.Is(postgres.ConfigureBacklog(f.ctx, f.a, limits), submission.ErrInvalid) {
				t.Fatal("invalid policy accepted")
			}
		}
		var policies int
		must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM submission_backlog_policy`).Scan(&policies))
		if policies != 0 {
			t.Fatal("invalid administration initialized a hidden default")
		}
		backlogCounts(t, f, 0)
		backlogPolicy(t, f, 1, 1)
		row, created, err := f.first.Accept(f.ctx, "user", "training", "backlog-historical-01", f.request)
		must(t, err)
		if !created {
			t.Fatal("new request not created")
		}
		_, err = f.a.Exec(f.ctx, `DELETE FROM submission_backlog_policy`)
		must(t, err)
		retry, created, err := f.second.Accept(f.ctx, "user", "training", "backlog-historical-01", f.request)
		must(t, err)
		if created || retry.ID != row.ID {
			t.Fatal("missing policy hid historical acceptance")
		}
		changed := f.request
		changed.Source += "// changed"
		_, _, err = f.second.Accept(f.ctx, "user", "training", "backlog-historical-01", changed)
		if !errors.Is(err, submission.ErrConflict) {
			t.Fatal("historical conflict changed")
		}
		_, _, err = f.first.Accept(f.ctx, "other", "training", "backlog-no-policy-02", f.request)
		if !errors.Is(err, submission.ErrAdmissionPolicy) {
			t.Fatal("new request bypassed missing policy")
		}
		backlogCounts(t, f, 1)
	})
	for _, tc := range []struct {
		name            string
		global, subject int64
		subjects        [2]string
	}{{"global last slot", 1, 1, [2]string{"a", "b"}}, {"subject spans contexts", 4, 1, [2]string{"a", "a"}}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBacklogFixture(t)
			backlogPolicy(t, f, tc.global, tc.subject)
			type result struct {
				row     submission.Receipt
				created bool
				err     error
			}
			results := make(chan result, 2)
			start := make(chan struct{})
			for i, s := range []*postgres.Store{f.first, f.second} {
				go func(i int, s *postgres.Store) {
					<-start
					row, created, err := s.Accept(f.ctx, tc.subjects[i], []string{"training-a", "training-b"}[i], []string{"backlog-compete-01", "backlog-compete-02"}[i], f.request)
					results <- result{row, created, err}
				}(i, s)
			}
			close(start)
			success, denied := 0, 0
			for range 2 {
				r := <-results
				if r.err == nil && r.created {
					success++
				} else if errors.Is(r.err, submission.ErrBacklog) {
					denied++
				} else {
					t.Fatalf("unexpected result: created=%v err=%v", r.created, r.err)
				}
			}
			if success != 1 || denied != 1 {
				t.Fatalf("success=%d denied=%d", success, denied)
			}
			backlogCounts(t, f, 1)
		})
	}
	for _, rollback := range []bool{false, true} {
		name := "waiter sees committed acceptance"
		if rollback {
			name = "rollback leaves capacity"
		}
		t.Run(name, func(t *testing.T) {
			f := newBacklogFixture(t)
			backlogPolicy(t, f, 1, 1)
			_, err := f.a.Exec(f.ctx, `CREATE TABLE backlog_fault(fail_commit boolean); CREATE FUNCTION pause_backlog_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
                IF EXISTS(SELECT 1 FROM submissions WHERE id=NEW.submission_id AND subject_id='paused-writer') THEN
                    PERFORM pg_advisory_xact_lock(91465067);
                    IF (SELECT fail_commit FROM backlog_fault) THEN RAISE EXCEPTION 'synthetic transaction failure'; END IF;
                END IF; RETURN NEW; END; $$;
                CREATE TRIGGER pause_backlog AFTER INSERT ON judge_tasks FOR EACH ROW EXECUTE FUNCTION pause_backlog_insert()`)
			must(t, err)
			_, err = f.a.Exec(f.ctx, `INSERT INTO backlog_fault VALUES($1)`, rollback)
			must(t, err)
			gate, err := f.a.Acquire(f.ctx)
			must(t, err)
			defer func() {
				cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = gate.Conn().Close(cleanup)
				gate.Release()
			}()
			_, err = gate.Exec(f.ctx, `SELECT pg_advisory_lock(91465067)`)
			must(t, err)
			writer := make(chan error, 1)
			waiter := make(chan error, 1)
			go func() {
				_, _, err := f.first.Accept(f.ctx, "paused-writer", "training", "backlog-locked-writer", f.request)
				writer <- err
			}()
			pid := waitBacklogBlocked(t, f, gate.Conn().PgConn().PID(), f.appA)
			go func() {
				_, _, err := f.second.Accept(f.ctx, "waiting-user", "training", "backlog-locked-waiter", f.request)
				waiter <- err
			}()
			waitBacklogBlocked(t, f, pid, f.appB)
			_, err = gate.Exec(f.ctx, `SELECT pg_advisory_unlock(91465067)`)
			must(t, err)
			writerErr, waiterErr := <-writer, <-waiter
			if rollback {
				if writerErr == nil || waiterErr != nil {
					t.Fatalf("rollback writer=%v waiter=%v", writerErr, waiterErr)
				}
			} else if writerErr != nil || !errors.Is(waiterErr, submission.ErrBacklog) {
				t.Fatalf("commit writer=%v waiter=%v", writerErr, waiterErr)
			}
			backlogCounts(t, f, 1)
		})
	}
	t.Run("full same-key races preserve receipt and conflict", func(t *testing.T) {
		f := newBacklogFixture(t)
		backlogPolicy(t, f, 1, 1)
		type result struct {
			row     submission.Receipt
			created bool
			err     error
		}
		results := make(chan result, 2)
		for _, s := range []*postgres.Store{f.first, f.second} {
			go func(s *postgres.Store) {
				r, c, e := s.Accept(f.ctx, "same-user", "training", "backlog-same-key-01", f.request)
				results <- result{r, c, e}
			}(s)
		}
		a, b := <-results, <-results
		must(t, a.err)
		must(t, b.err)
		if a.row.ID != b.row.ID || a.created == b.created {
			t.Fatal("concurrent identical requests consumed twice")
		}
		changed := f.request
		changed.Source += "// different"
		_, _, err := f.second.Accept(f.ctx, "same-user", "training", "backlog-same-key-01", changed)
		if !errors.Is(err, submission.ErrConflict) {
			t.Fatal("quota replaced original conflict")
		}
		backlogCounts(t, f, 1)
	})
	t.Run("lock cancellation is unknown not quota", func(t *testing.T) {
		f := newBacklogFixture(t)
		backlogPolicy(t, f, 1, 1)
		held, err := f.a.Begin(f.ctx)
		must(t, err)
		defer held.Rollback(context.Background())
		_, err = held.Exec(f.ctx, `SELECT * FROM submission_backlog_policy FOR UPDATE`)
		must(t, err)
		ctx, cancel := context.WithCancel(f.ctx)
		defer cancel()
		result := make(chan error, 1)
		go func() {
			_, _, err := f.second.Accept(ctx, "user", "training", "backlog-cancelled-01", f.request)
			result <- err
		}()
		waitBacklogBlocked(t, f, held.Conn().PgConn().PID(), f.appB)
		cancel()
		err = <-result
		if err == nil || errors.Is(err, submission.ErrBacklog) || errors.Is(err, submission.ErrAdmissionPolicy) {
			t.Fatalf("lock failure misclassified: %v", err)
		}
		must(t, held.Rollback(f.ctx))
		backlogCounts(t, f, 0)
	})
	t.Run("administrator can lower shared limits without deleting work", func(t *testing.T) {
		f := newBacklogFixture(t)
		backlogPolicy(t, f, 3, 3)
		for _, key := range []string{"backlog-lower-existing-01", "backlog-lower-existing-02"} {
			_, _, err := f.first.Accept(f.ctx, "user", "training", key, f.request)
			must(t, err)
		}
		backlogPolicy(t, f, 1, 1)
		_, _, err := f.second.Accept(f.ctx, "other", "another", "backlog-lower-rejected-01", f.request)
		if !errors.Is(err, submission.ErrBacklog) {
			t.Fatal("replica ignored lower shared policy")
		}
		_, created, err := f.second.Accept(f.ctx, "user", "training", "backlog-lower-existing-01", f.request)
		must(t, err)
		if created {
			t.Fatal("existing receipt recreated")
		}
		backlogCounts(t, f, 2)
	})
}

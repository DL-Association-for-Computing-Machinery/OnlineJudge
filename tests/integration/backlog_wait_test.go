//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

func TestBacklogWaitingReceiptPriority(t *testing.T) {
	f := newBacklogFixture(t)
	backlogPolicy(t, f, 1, 1)
	_, err := f.a.Exec(f.ctx, `CREATE FUNCTION pause_receipt_writer() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
        PERFORM pg_advisory_xact_lock(91465068); RETURN NEW; END; $$;
        CREATE TRIGGER pause_receipt AFTER INSERT ON judge_tasks FOR EACH ROW EXECUTE FUNCTION pause_receipt_writer()`)
	must(t, err)
	gate, err := f.a.Acquire(f.ctx)
	must(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = gate.Conn().Close(ctx)
		gate.Release()
	}()
	_, err = gate.Exec(f.ctx, `SELECT pg_advisory_lock(91465068)`)
	must(t, err)
	type result struct {
		row     submission.Receipt
		created bool
		err     error
	}
	writer, waiter := make(chan result, 1), make(chan result, 1)
	go func() {
		r, c, e := f.first.Accept(f.ctx, "user", "training", "backlog-wait-same-key", f.request)
		writer <- result{r, c, e}
	}()
	pid := waitBacklogBlocked(t, f, gate.Conn().PgConn().PID(), f.appA)
	// Accepted source remains immutable while a later material failure must not
	// hide its receipt from a same-key request already waiting on the policy.
	_, err = f.a.Exec(f.ctx, `UPDATE submission_materials SET manifest='{}'::jsonb WHERE version_id=$1`, f.request.MaterialVersion)
	must(t, err)
	go func() {
		r, c, e := f.second.Accept(f.ctx, "user", "training", "backlog-wait-same-key", f.request)
		waiter <- result{r, c, e}
	}()
	waitBacklogBlocked(t, f, pid, f.appB)
	_, err = gate.Exec(f.ctx, `SELECT pg_advisory_unlock(91465068)`)
	must(t, err)
	a, b := <-writer, <-waiter
	must(t, a.err)
	must(t, b.err)
	if !a.created || b.created || a.row.ID != b.row.ID {
		t.Fatal("waiting identical request did not retain original receipt")
	}
	backlogCounts(t, f, 1)
}

func TestBacklogWaitingConfiguration(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(fmt.Sprintf("lower policy commit=%v", commit), func(t *testing.T) {
			f := newBacklogFixture(t)
			backlogPolicy(t, f, 2, 2)
			_, _, err := f.first.Accept(f.ctx, "first", "training", "backlog-admin-first", f.request)
			must(t, err)
			held, err := f.a.Begin(f.ctx)
			must(t, err)
			defer held.Rollback(context.Background())
			_, err = held.Exec(f.ctx, `UPDATE submission_backlog_policy SET global_limit=1,subject_limit=1`)
			must(t, err)
			result := make(chan error, 1)
			go func() {
				_, _, err := f.second.Accept(f.ctx, "second", "training", "backlog-admin-waiter", f.request)
				result <- err
			}()
			waitBacklogBlocked(t, f, held.Conn().PgConn().PID(), f.appB)
			if commit {
				must(t, held.Commit(f.ctx))
			} else {
				must(t, held.Rollback(f.ctx))
			}
			err = <-result
			if commit {
				if !errors.Is(err, submission.ErrBacklog) {
					t.Fatalf("waiter ignored lowered policy: %v", err)
				}
				backlogCounts(t, f, 1)
			} else {
				must(t, err)
				backlogCounts(t, f, 2)
			}
		})
	}
}

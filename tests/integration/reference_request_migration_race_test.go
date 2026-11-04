//go:build integration

package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type referenceBeforeDraft struct {
	referencepg.DraftAccess
	entered chan uint32
	release <-chan struct{}
}

func (a referenceBeforeDraft) LockOwnDraftIdentity(ctx context.Context, tx pgx.Tx, actor identity.ProblemActor, id string) (problem.DraftIdentity, error) {
	a.entered <- tx.Conn().PgConn().PID()
	select {
	case <-a.release:
	case <-ctx.Done():
		return problem.DraftIdentity{}, ctx.Err()
	}
	return a.DraftAccess.LockOwnDraftIdentity(ctx, tx, actor, id)
}

func TestReferenceRequestRollbackRace(t *testing.T) {
	f := newReferenceRequestFixture(t)
	f.limits(t, 1, 1)
	base, err := problempg.NewReferenceAccess(f.access)
	must(t, err)
	gate := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	defer release()
	held := referenceBeforeDraft{DraftAccess: base, entered: make(chan uint32, 1), release: gate}
	store, err := referencepg.New(f.a, held)
	must(t, err)
	accepted := make(chan error, 1)
	go func() {
		_, e := store.Enqueue(f.ctx, f.actor, "reference-rollback-race-key", f.draft.ID, f.catalog)
		accepted <- e
	}()
	var writer uint32
	select {
	case writer = <-held.entered:
	case e := <-accepted:
		t.Fatalf("writer stopped before draft gate: %v", e)
	case <-f.ctx.Done():
		t.Fatal("writer did not reach draft gate")
	}
	rolledBack := make(chan error, 1)
	go func() { _, e := f.provider.Down(f.ctx); rolledBack <- e }()
	// The writer has read requests after locking current authority. Down must
	// wait for it before acquiring later request/draft table locks.
	watch, cancel := context.WithTimeout(f.ctx, 2*time.Second)
	defer cancel()
	for {
		var blocked bool
		must(t, f.b.QueryRow(watch, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)))`, writer).Scan(&blocked))
		if blocked {
			break
		}
		select {
		case <-watch.Done():
			t.Fatal("rollback did not wait on the actual writer")
		case <-time.After(5 * time.Millisecond):
		}
	}
	release()
	must(t, <-accepted)
	err = <-rolledBack
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "P0001" || pgerr.Message != "cannot discard reference requests or configured admission; use a forward migration" {
		t.Fatalf("rollback did not safely refuse retained history: %v", err)
	}
	must(t, db.CheckSchema(f.ctx, f.a))
	f.counts(t, 1, 1)
}

func TestReferenceRequestEmptyRollbackAfterAuthor(t *testing.T) {
	f := newReferenceRequestFixture(t)
	// No configured policy or requests: Down must reach DROP, including removal
	// of FK triggers on credentials. A nonempty policy would mask this lock edge.
	base, err := problempg.NewReferenceAccess(f.access)
	must(t, err)
	gate := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	defer release()
	held := heldReferenceAccess{DraftAccess: base, locked: make(chan uint32, 1), release: gate}
	store, err := referencepg.New(f.a, held)
	must(t, err)
	type outcome struct {
		r   problemreference.Request
		err error
	}
	accepted := make(chan outcome, 1)
	go func() {
		r, e := store.Enqueue(f.ctx, f.actor, "reference-empty-rollback-key", f.draft.ID, f.catalog)
		accepted <- outcome{r, e}
	}()
	var writer uint32
	select {
	case writer = <-held.locked:
	case r := <-accepted:
		t.Fatalf("writer stopped before author gate: %v", r.err)
	case <-f.ctx.Done():
		t.Fatal("author gate timed out")
	}
	rolledBack := make(chan error, 1)
	go func() { _, e := f.provider.Down(f.ctx); rolledBack <- e }()
	watch, cancel := context.WithTimeout(f.ctx, 2*time.Second)
	defer cancel()
	for {
		var blocked bool
		must(t, f.b.QueryRow(watch, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)))`, writer).Scan(&blocked))
		if blocked {
			break
		}
		select {
		case <-watch.Done():
			t.Fatal("empty rollback did not wait on actual author")
		case <-time.After(5 * time.Millisecond):
		}
	}
	release()
	result := <-accepted
	var pgerr *pgconn.PgError
	if errors.As(result.err, &pgerr) {
		t.Logf("admission SQLSTATE=%s", pgerr.Code)
	}
	noReferenceResult(t, result.r, result.err, problemreference.ErrPolicy)
	must(t, <-rolledBack)
	if db.CheckSchema(f.ctx, f.a) == nil {
		t.Fatal("schema gate accepted rolled-back schema8")
	}
	var version int
	must(t, f.a.QueryRow(f.ctx, `SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1`).Scan(&version))
	if version != 8 {
		t.Fatal("empty rollback did not reach schema8")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
	f.counts(t, 0, 0)
}

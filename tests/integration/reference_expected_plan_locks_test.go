//go:build integration && linux

package integration_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestExpectedPlanDown10WaitsForReaderAndWriter(t *testing.T) {
	for _, write := range []bool{false, true} {
		name := "reader"
		if write {
			name = "writer"
		}
		t.Run(name, func(t *testing.T) {
			f := newExpectedPlanFixture(t)
			_, err := f.provider.DownTo(f.ctx, 10)
			must(t, err)
			holder, err := f.b.Begin(f.ctx)
			must(t, err)
			defer func() { _ = holder.Rollback(f.ctx) }()
			_, err = holder.Exec(f.ctx, `LOCK TABLE problem_reference_expected_plans IN ACCESS EXCLUSIVE MODE`)
			must(t, err)
			s := f.service(t, f.session, f.store)
			finished := make(chan error, 1)
			go func() {
				if write {
					_, err := s.Prepare(f.ctx, f.credential, expectedPlanKey)
					finished <- err
				} else {
					_, err := s.Receipt(f.ctx, f.credential, expectedPlanKey)
					finished <- err
				}
			}()
			pid := waitBacklogBlocked(t, f.backlogFixture, holder.Conn().PgConn().PID(), f.appA)
			down := make(chan error, 1)
			go func() { _, err := f.provider.DownTo(f.ctx, 9); down <- err }()
			watch, cancel := context.WithTimeout(f.ctx, 2*time.Second)
			defer cancel()
			for {
				var blocked bool
				must(t, f.a.QueryRow(watch, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
				    WHERE $1::integer=ANY(pg_blocking_pids(pid))
				    AND query LIKE '%LOCK TABLE problem_reference_requests, problem_reference_expected_plans%')`, pid).Scan(&blocked))
				if blocked {
					break
				}
				select {
				case err := <-down:
					t.Fatalf("Down10 did not wait on the active plan transaction: %v", err)
				case <-watch.Done():
					t.Fatal("actual Down10 lock wait not observed")
				case <-time.After(5 * time.Millisecond):
				}
			}
			must(t, holder.Commit(f.ctx))
			if write {
				must(t, <-finished)
				var e *pgconn.PgError
				if err := <-down; !errors.As(err, &e) || e.Code != "P0001" || !strings.Contains(e.Message, "cannot discard expected reference plans") {
					t.Fatalf("Down10 did not preserve the committed plan: %v", err)
				}
				_, err := f.provider.Up(f.ctx)
				must(t, err)
				must(t, db.CheckSchema(f.ctx, f.a))
				f.planCount(t, 1)
			} else {
				if err := <-finished; !errors.Is(err, problemreference.ErrNotFound) {
					t.Fatalf("read did not finish before Down10: %v", err)
				}
				must(t, <-down)
				if db.CheckSchema(f.ctx, f.a) == nil {
					t.Fatal("startup accepted schema 9 after Down10")
				}
			}
		})
	}
}

func TestExpectedPlanWriteHoldsAuthorLockUntilCommit(t *testing.T) {
	f := newExpectedPlanFixture(t)
	holder, err := f.b.Begin(f.ctx)
	must(t, err)
	defer func() { _ = holder.Rollback(f.ctx) }()
	_, err = holder.Exec(f.ctx, `SELECT id FROM problem_reference_requests WHERE id=$1 FOR UPDATE`, f.request.ID)
	must(t, err)
	s := f.service(t, f.session, f.store)
	wrote := make(chan error, 1)
	go func() {
		_, err := s.Prepare(f.ctx, f.credential, expectedPlanKey)
		wrote <- err
	}()
	writer := waitBacklogBlocked(t, f.backlogFixture, holder.Conn().PgConn().PID(), f.appA)
	revoked := make(chan error, 1)
	go func() {
		_, err := f.access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: f.actor.SubjectID,
			ExpectedRevision: 1, Enabled: false, Operator: "expected-plan-fixture", Reason: "test transactional authority"})
		revoked <- err
	}()
	waitBacklogBlocked(t, f.backlogFixture, writer, f.appA)
	must(t, holder.Commit(f.ctx))
	must(t, <-wrote)
	must(t, <-revoked)
	plan, err := s.Receipt(f.ctx, f.credential, expectedPlanKey)
	noExpectedPlan(t, plan, err, identity.ErrForbidden)
	f.planCount(t, 1)
}

func TestExpectedPlanDatabaseAndReadbackIntegrity(t *testing.T) {
	f := newExpectedPlanFixture(t)
	const insert = `INSERT INTO problem_reference_expected_plans VALUES ($1,$2,$3)`
	large := bytes.Repeat([]byte{'x'}, problemreference.MaxExpectedPlanBytes+1)
	for _, tc := range []struct {
		id, digest, code string
		raw              []byte
	}{
		{f.request.ID, strings.Repeat("a", 64), "23514", f.want.Encoding},
		{f.request.ID, problemfixture.Hash(large), "23514", large},
		{identityID(9999), f.want.SHA256, "23503", f.want.Encoding},
	} {
		_, err := f.a.Exec(f.ctx, insert, tc.id, tc.raw, tc.digest)
		var e *pgconn.PgError
		if !errors.As(err, &e) || e.Code != tc.code {
			t.Fatalf("expected integrity SQLSTATE %s, got %v", tc.code, err)
		}
	}
	// Even valid hash bytes cannot make a different import or noncanonical
	// representation readable as the current request's expected plan.
	noncanonical := append(bytes.Clone(f.want.Encoding), '\n')
	wrongImport := bytes.Replace(f.want.Encoding, []byte(f.request.Specification.Identity.SourceLockSHA256), bytes.Repeat([]byte{'a'}, 64), 1)
	_, err := f.a.Exec(f.ctx, insert, f.request.ID, noncanonical, problemfixture.Hash(noncanonical))
	must(t, err)
	s := f.service(t, f.session, f.store)
	plan, err := s.Receipt(f.ctx, f.credential, expectedPlanKey)
	noExpectedPlan(t, plan, err, problemreference.ErrStore)
	_, err = f.a.Exec(f.ctx, `UPDATE problem_reference_expected_plans SET plan_encoding=$1,plan_sha256=$2 WHERE request_id=$3`,
		wrongImport, problemfixture.Hash(wrongImport), f.request.ID)
	must(t, err)
	plan, err = s.Receipt(f.ctx, f.credential, expectedPlanKey)
	noExpectedPlan(t, plan, err, problemreference.ErrStore)
}

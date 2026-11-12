//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func TestTrainingVirtualTimeAcrossStores(t *testing.T) {
	f := newBacklogFixture(t)
	backlogPolicy(t, f, 100, 100)
	nodes := []submission.NodeIdentity{{ID: "fairness-a", CredentialVersion: 1}, {ID: "fairness-b", CredentialVersion: 1}}
	for _, node := range nodes {
		_, err := f.a.Exec(f.ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,$2)`, node.ID, node.CredentialVersion)
		must(t, err)
	}
	accept := func(s *postgres.Store, subject, scope string, position int) submission.Receipt {
		t.Helper()
		r, created, err := s.Accept(f.ctx, subject, scope, fmt.Sprintf("fairness-fixture-%04d", position), f.request)
		must(t, err)
		if !created {
			t.Fatal("new submission did not get its own task")
		}
		return r
	}
	hot := make([]submission.Receipt, 6)
	for i := range hot {
		if i%2 == 0 {
			hot[i] = accept(f.first, "hot", "training-left", i+1)
		} else {
			hot[i] = accept(f.second, "hot", "training-right", i+1)
		}
	}
	cold := accept(f.second, "cold", "training-right", 7)
	report := submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("c", 64)}
	for i, expected := range []submission.Receipt{hot[0], cold, hot[1], hot[2], hot[3], hot[4], hot[5]} {
		store, node := f.first, nodes[0]
		if i%2 != 0 {
			store, node = f.second, nodes[1]
		}
		lease, err := store.Claim(f.ctx, node)
		must(t, err)
		if lease.SubmissionID != expected.ID {
			got := "unknown submission"
			for j, h := range hot {
				if lease.SubmissionID == h.ID {
					got = fmt.Sprintf("hot submission %d", j+1)
				}
			}
			if lease.SubmissionID == cold.ID {
				got = "cold submission"
			}
			t.Fatalf("unexpected task in claim position %d: %s", i+1, got)
		}
		must(t, store.Report(f.ctx, node, lease.Token, report))
	}
}

func TestTrainingClockContentionAndRetry(t *testing.T) {
	f := newBacklogFixture(t)
	backlogPolicy(t, f, 100, 100)
	nodeA := submission.NodeIdentity{ID: "train-node-a", CredentialVersion: 1}
	nodeB := submission.NodeIdentity{ID: "train-node-b", CredentialVersion: 1}
	_, err := f.a.Exec(f.ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,1),($2,1)`, nodeA.ID, nodeB.ID)
	must(t, err)
	otherRequest := f.request
	otherRequest.MaterialVersion = "another-training-version"
	other, err := postgres.New(f.a, otherRequest.MaterialVersion, []byte(`{"training":"other"}`))
	must(t, err)
	must(t, other.RegisterFixture(f.ctx))
	must(t, other.RegisterFixture(f.ctx))

	first, _, err := f.first.Accept(f.ctx, "hot", "left", "training-first-001", f.request)
	must(t, err)
	second, _, err := f.first.Accept(f.ctx, "hot", "right", "training-second-01", f.request)
	must(t, err)
	foreign, _, err := other.Accept(f.ctx, "hot", "left", "training-foreign-1", otherRequest)
	must(t, err)
	held, err := f.a.Begin(f.ctx)
	must(t, err)
	defer held.Rollback(context.Background())
	_, err = held.Exec(f.ctx, `SELECT 1 FROM judge_tasks WHERE submission_id=$1 FOR UPDATE`, first.ID)
	must(t, err)
	type claimResult struct {
		lease submission.Lease
		err   error
	}
	claimed := make(chan claimResult, 1)
	go func() {
		lease, err := f.second.Claim(f.ctx, nodeB)
		claimed <- claimResult{lease, err}
	}()
	firstClaimerPID := waitBacklogBlocked(t, f, held.Conn().PgConn().PID(), f.appB)
	// Version B can claim while version A is blocked by its own queue head.
	foreignLease, err := other.Claim(f.ctx, nodeA)
	must(t, err)
	if foreignLease.SubmissionID != foreign.ID {
		t.Fatal("claim crossed material versions")
	}
	ac := submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("c", 64)}
	must(t, other.Report(f.ctx, nodeA, foreignLease.Token, ac))
	secondClaim := make(chan claimResult, 1)
	go func() {
		lease, err := f.first.Claim(f.ctx, nodeA)
		secondClaim <- claimResult{lease, err}
	}()
	waitBacklogBlocked(t, f, firstClaimerPID, f.appA)
	must(t, held.Commit(f.ctx))
	result := <-claimed
	must(t, result.err)
	if result.lease.SubmissionID != first.ID {
		t.Fatal("a later task passed the temporarily locked head")
	}
	later := <-secondClaim
	must(t, later.err)
	if later.lease.SubmissionID != second.ID {
		t.Fatal("claim waiting for the clock did not see the preceding claim")
	}
	must(t, f.first.Report(f.ctx, nodeA, later.lease.Token, ac))
	se := submission.Result{Verdict: "SE", EvidenceSHA256: strings.Repeat("d", 64)}
	must(t, f.second.Report(f.ctx, nodeB, result.lease.Token, se))
	retry, err := f.first.Claim(f.ctx, nodeA)
	must(t, err)
	if retry.SubmissionID != first.ID || retry.AttemptNumber != result.lease.AttemptNumber+1 {
		t.Fatal("retry lost its original place or attempt sequence")
	}
	must(t, f.second.Report(f.ctx, nodeB, result.lease.Token, se))
	must(t, f.first.Report(f.ctx, nodeA, retry.Token, ac))
	if _, err := f.first.Claim(f.ctx, nodeA); !errors.Is(err, submission.ErrNoTask) {
		t.Fatalf("empty queue claim: %v", err)
	}
	replayed, created, err := f.second.Accept(f.ctx, "hot", "left", "training-first-001", f.request)
	must(t, err)
	if created || replayed.ID != first.ID {
		t.Fatal("idempotent replay created a new queue entry")
	}
	fresh, _, err := f.second.Accept(f.ctx, "new", "right", "training-fresh-001", f.request)
	must(t, err)
	var floor, rank, foreignFloor, originalRank, order int64
	must(t, f.a.QueryRow(f.ctx, `SELECT c.floor,t.virtual_finish FROM training_queue_clock c
		JOIN judge_tasks t ON t.queue_version=c.version_id WHERE t.submission_id=$1`, fresh.ID).Scan(&floor, &rank))
	must(t, f.a.QueryRow(f.ctx, `SELECT floor FROM training_queue_clock WHERE version_id=$1`, otherRequest.MaterialVersion).Scan(&foreignFloor))
	must(t, f.a.QueryRow(f.ctx, `SELECT virtual_finish,enqueue_order FROM judge_tasks WHERE submission_id=$1`, first.ID).Scan(&originalRank, &order))
	if floor != 2 || rank != 3 || foreignFloor != 1 || originalRank != 1 || order != 1 {
		t.Fatalf("floor=%d rank=%d foreign=%d original=(%d,%d)", floor, rank, foreignFloor, originalRank, order)
	}
}

func TestTrainingClockFailureIsAtomic(t *testing.T) {
	f := newBacklogFixture(t)
	backlogPolicy(t, f, 100, 100)
	node := submission.NodeIdentity{ID: "train-node-failure", CredentialVersion: 1}
	_, err := f.a.Exec(f.ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)`, node.ID)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `UPDATE training_queue_clock SET floor=$1`, int64(math.MaxInt64))
	must(t, err)
	if _, _, err := f.first.Accept(f.ctx, "hot", "left", "training-failure-1", f.request); err == nil {
		t.Fatal("exhausted clock accepted a submission")
	}
	backlogCounts(t, f, 0)
	_, err = f.a.Exec(f.ctx, `UPDATE training_queue_clock SET floor=0;
		CREATE FUNCTION reject_training_attempt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected attempt failure'; END $$;
		CREATE CONSTRAINT TRIGGER reject_training_attempt AFTER INSERT ON judge_attempts
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_training_attempt()`)
	must(t, err)
	r, created, err := f.first.Accept(f.ctx, "hot", "left", "training-failure-1", f.request)
	must(t, err)
	if !created {
		t.Fatal("failed admission consumed the key")
	}
	if _, err := f.first.Claim(f.ctx, node); err == nil || !strings.Contains(err.Error(), "injected attempt failure") {
		t.Fatalf("unexpected failed claim result: %v", err)
	}
	var floor int64
	var state string
	var attempts, attemptNumber int
	must(t, f.a.QueryRow(f.ctx, `SELECT c.floor,t.state,t.attempt_number,(SELECT count(*) FROM judge_attempts)
		FROM judge_tasks t JOIN training_queue_clock c ON c.version_id=t.queue_version WHERE t.submission_id=$1`, r.ID).Scan(&floor, &state, &attemptNumber, &attempts))
	if floor != 0 || state != "queued" || attemptNumber != 0 || attempts != 0 {
		t.Fatalf("failed claim changed clock or task: floor=%d state=%s attempt=%d attempts=%d", floor, state, attemptNumber, attempts)
	}
}

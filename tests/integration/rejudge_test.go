//go:build integration

package integration_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestControlledRejudge(t *testing.T) {
	f := newBacklogFixture(t)
	backlogPolicy(t, f, 2, 1)
	var database string
	must(t, f.a.QueryRow(f.ctx, "SELECT current_database()").Scan(&database))
	_, err := f.a.Exec(f.ctx, "CREATE ROLE oj_rejudge_operator NOLOGIN")
	must(t, err)
	t.Cleanup(func() {
		if _, err := f.a.Exec(f.ctx, "DROP ROLE oj_rejudge_operator"); err != nil {
			t.Error("could not remove the test-only role")
		}
	})
	node := submission.NodeIdentity{ID: "rejudge-node", CredentialVersion: 1}
	_, err = f.a.Exec(f.ctx, "INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)", node.ID)
	must(t, err)
	original, _, err := f.first.Accept(f.ctx, "owner", submission.PersonalTrainingScope, "original-rejudge-submission", f.request)
	must(t, err)
	oldLease, err := f.first.Claim(f.ctx, node)
	must(t, err)
	result := submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("b", 64)}
	must(t, f.first.Report(f.ctx, node, oldLease.Token, result))
	i := postgres.RejudgeIntent{ID: original.ID, Version: original.MaterialVersion, Digest: oldLease.ManifestSHA256,
		Key: original.ID + "-first", PreviousGeneration: 1, Reason: "check historical result"}
	verify := func(_ context.Context, version, digest string, raw []byte) error {
		if sumRaw, hash, err := postgres.CanonicalManifest(raw); err != nil || string(sumRaw) != string(raw) || hash != digest || version != i.Version {
			return errors.New("incorrect material supplied to verifier")
		}
		return nil
	}
	assert := func(want, got error) {
		t.Helper()
		if !errors.Is(got, want) {
			t.Fatalf("wanted %v, got %v", want, got)
		}
	}
	_, err = postgres.Rejudge(f.ctx, f.a, database, i, verify)
	assert(postgres.ErrRejudgeOperator, err)
	_, err = f.a.Exec(f.ctx, "GRANT oj_rejudge_operator TO oj_test")
	must(t, err)
	// Re-check authority after out-of-transaction file verification.
	_, err = postgres.Rejudge(f.ctx, f.a, database, i, func(context.Context, string, string, []byte) error {
		_, err := f.b.Exec(f.ctx, "REVOKE oj_rejudge_operator FROM oj_test")
		return err
	})
	assert(postgres.ErrRejudgeOperator, err)
	_, err = f.a.Exec(f.ctx, "GRANT oj_rejudge_operator TO oj_test")
	must(t, err)
	_, err = postgres.Rejudge(f.ctx, f.a, database+"-wrong", i, verify)
	assert(postgres.ErrRestoreDatabase, err)
	_, err = postgres.Rejudge(f.ctx, f.a, database, i, func(context.Context, string, string, []byte) error { return errors.New("missing") })
	assert(submission.ErrMaterial, err)
	_, err = postgres.ReadRejudge(f.ctx, f.a, database, i.Key)
	assert(submission.ErrNotFound, err)

	notTraining, _, err := f.first.Accept(f.ctx, "owner", "other-context", "not-a-training-rejudge", f.request)
	must(t, err)
	other := i
	other.ID = notTraining.ID
	_, err = postgres.Rejudge(f.ctx, f.a, database, other, verify)
	assert(postgres.ErrRejudgeState, err)
	otherLease, err := f.first.Claim(f.ctx, node)
	must(t, err)
	if otherLease.SubmissionID != notTraining.ID {
		t.Fatal("unexpected control submission claimed")
	}
	must(t, f.first.Report(f.ctx, node, otherLease.Token, result))
	_, err = postgres.Rejudge(f.ctx, f.a, database, other, verify)
	assert(postgres.ErrRejudgeState, err)

	queued, _, err := f.first.Accept(f.ctx, "owner", submission.PersonalTrainingScope, "queued-before-rejudge", f.request)
	must(t, err)
	_, err = postgres.Rejudge(f.ctx, f.b, database, i, verify)
	assert(submission.ErrBacklog, err)
	backlogPolicy(t, f, 3, 3)
	results := make(chan struct {
		key string
		r   postgres.RejudgeReceipt
		err error
	}, 3)
	var ready, done sync.WaitGroup
	ready.Add(3)
	for n, pool := range []*pgxpool.Pool{f.a, f.b, f.a} {
		done.Go(func() {
			req := i
			if n == 2 {
				req.Key = original.ID + "-second"
			}
			ready.Done()
			ready.Wait()
			r, err := postgres.Rejudge(f.ctx, pool, database, req, verify)
			results <- struct {
				key string
				r   postgres.RejudgeReceipt
				err error
			}{req.Key, r, err}
		})
	}
	done.Wait()
	close(results)
	var winner postgres.RejudgeReceipt
	for x := range results {
		if x.err != nil {
			assert(postgres.ErrRejudgeState, x.err)
			continue
		}
		if x.r.Key != x.key || x.r.SubmissionID != original.ID || x.r.Generation != 2 || x.r.Operator != "oj_test" ||
			(!winner.RequestedAt.IsZero() && x.r != winner) {
			t.Fatal("concurrent rejudges did not return one immutable receipt")
		}
		winner = x.r
	}
	if winner.RequestedAt.IsZero() {
		t.Fatal("no rejudge succeeded")
	}
	audit, err := postgres.ReadRejudge(f.ctx, f.b, database, winner.Key)
	must(t, err)
	if audit != winner {
		t.Fatal("readback did not return the original audit")
	}
	conflict := i
	conflict.Key, conflict.Reason = winner.Key, "changed reason"
	_, err = postgres.Rejudge(f.ctx, f.a, database, conflict, verify)
	assert(submission.ErrConflict, err)
	replay := i
	replay.Key = winner.Key
	replayed, err := postgres.Rejudge(f.ctx, f.a, database, replay, func(context.Context, string, string, []byte) error {
		return errors.New("materials unavailable after committed audit")
	})
	must(t, err)
	if replayed != winner {
		t.Fatal("exact replay did not return recorded audit")
	}
	assert(submission.ErrLease, f.first.Report(f.ctx, node, oldLease.Token, result))
	_, err = f.first.Renew(f.ctx, node, oldLease.Token)
	assert(submission.ErrLease, err)
	next, err := f.first.Claim(f.ctx, node)
	must(t, err)
	if next.SubmissionID != queued.ID {
		t.Fatal("the rejudge skipped previously queued work")
	}
	must(t, f.first.Report(f.ctx, node, next.Token, result))
	next, err = f.first.Claim(f.ctx, node)
	must(t, err)
	if next.SubmissionID != original.ID || next.AttemptNumber != 1 || next.JudgeGeneration != 2 ||
		next.MaterialVersion != oldLease.MaterialVersion || next.ManifestSHA256 != oldLease.ManifestSHA256 {
		t.Fatal("new attempt changed original binding or did not advance generation")
	}
	must(t, f.first.Report(f.ctx, node, next.Token, result))
	var oldVerdict string
	must(t, f.a.QueryRow(f.ctx, "SELECT result_payload->>'Verdict' FROM judge_attempts WHERE token=$1 AND judge_generation=1", oldLease.Token).Scan(&oldVerdict))
	if oldVerdict != "AC" {
		t.Fatal("old result was overwritten")
	}
	var count int
	must(t, f.a.QueryRow(f.ctx, "SELECT count(*) FROM submission_rejudges").Scan(&count))
	if count != 1 {
		t.Fatal("expected exactly one audit record")
	}
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, "SET LOCAL enable_seqscan=off")
	must(t, err)
	rows, err := tx.Query(f.ctx, "EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM submission_rejudges WHERE operator=$1 AND request_key=$2",
		winner.Operator, winner.Key)
	must(t, err)
	var plan strings.Builder
	for rows.Next() {
		var line string
		must(t, rows.Scan(&line))
		plan.WriteString(line + "\n")
	}
	must(t, rows.Err())
	rows.Close()
	if !strings.Contains(plan.String(), "submission_rejudges_pkey") {
		t.Fatal("receipt lookup did not use the audit identity index with sequential scans disabled")
	}
	t.Log(plan.String())
	must(t, tx.Rollback(f.ctx))
	if _, err := f.provider.Down(f.ctx); err == nil {
		t.Fatal("rollback discarded durable rejudge history")
	}
}

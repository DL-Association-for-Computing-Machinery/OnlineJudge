//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func TestJudgeOperationsSnapshot(t *testing.T) {
	f := newBacklogFixture(t)
	backlogPolicy(t, f, 100, 100)
	var database string
	must(t, f.a.QueryRow(f.ctx, "SELECT current_database()").Scan(&database))
	read := func() postgres.JudgeOperationsSnapshot {
		t.Helper()
		got, err := postgres.ObserveJudgeOperations(f.ctx, f.b, database, 10)
		must(t, err)
		return got
	}
	empty := read()
	if empty.Queued != 0 || empty.Running != 0 || empty.OldestQueuedAcceptedAt != nil ||
		empty.RecentlyAccepted != 0 || empty.ConfiguredEnabledNodes != 0 || empty.ConfiguredEnabledSlots != 0 ||
		empty.NodesWithCurrentLease != 0 || empty.EnabledNodesWithoutCurrentLease != 0 {
		t.Fatalf("fresh observation not empty: %+v", empty)
	}
	if got, err := postgres.ObserveJudgeOperations(f.ctx, f.b, database+"-wrong", 10); !errors.Is(err, postgres.ErrJudgeSnapshotUnavailable) || !got.ObservedAt.IsZero() {
		t.Fatal("mismatched database returned a snapshot")
	}
	cancelled, cancel := context.WithCancel(f.ctx)
	cancel()
	if got, err := postgres.ObserveJudgeOperations(cancelled, f.b, database, 10); !errors.Is(err, postgres.ErrJudgeSnapshotUnavailable) || !got.ObservedAt.IsZero() {
		t.Fatal("cancelled observation returned a result")
	}
	const nodeID = "private-fixture-node"
	const disabledID = "disabled-fixture-node"
	const idleID = "idle-fixture-node"
	const subject = "private-fixture-subject"
	_, err := f.a.Exec(f.ctx, "INSERT INTO worker_nodes(id, credential_version, enabled, capacity) VALUES ($1, 1, true, 4), ($2, 1, false, 8), ($3, 1, true, 2)", nodeID, disabledID, idleID)
	must(t, err)
	node := submission.NodeIdentity{ID: nodeID, CredentialVersion: 1}
	accept := func(key string) submission.Receipt {
		t.Helper()
		result, _, err := f.first.Accept(f.ctx, subject, "m0-training", key, f.request)
		must(t, err)
		return result
	}
	report := func(lease submission.Lease, verdict string) {
		t.Helper()
		must(t, f.first.Report(f.ctx, node, lease.Token, submission.Result{Verdict: verdict, EvidenceSHA256: strings.Repeat("a", 64)}))
	}
	claim := func(id string) submission.Lease {
		t.Helper()
		lease, err := f.first.Claim(f.ctx, node)
		must(t, err)
		if lease.SubmissionID != id {
			t.Fatal("unexpected fixture lease")
		}
		return lease
	}
	failed := accept("snapshot-recovery-failed")
	for range 3 {
		report(claim(failed.ID), "SE")
	}
	finished := accept("snapshot-system-retried")
	report(claim(finished.ID), "SE")
	report(claim(finished.ID), "AC")
	healthy := accept("snapshot-current-lease")
	current := claim(healthy.ID)
	second := accept("snapshot-second-lease-on-same-node")
	claim(second.ID)
	expired := accept("snapshot-expired-lease")
	stale := claim(expired.ID)
	_, err = f.a.Exec(f.ctx, "UPDATE judge_attempts SET node_id=$1, lease_until=statement_timestamp()-interval '1 minute' WHERE token=$2", idleID, stale.Token)
	must(t, err)
	revoked := accept("snapshot-disabled-node")
	revokedLease := claim(revoked.ID)
	_, err = f.a.Exec(f.ctx, "UPDATE judge_attempts SET node_id=$1 WHERE token=$2", disabledID, revokedLease.Token)
	must(t, err)
	oldest := accept("snapshot-oldest-queued")
	_, err = f.a.Exec(f.ctx, "UPDATE submissions SET created_at=statement_timestamp()-interval '20 minutes' WHERE id=$1", oldest.ID)
	must(t, err)
	var acceptedAt time.Time
	must(t, f.a.QueryRow(f.ctx, "SELECT created_at FROM submissions WHERE id=$1", oldest.ID).Scan(&acceptedAt))
	want := read()
	if want.Schema != "oj.judge-operations.v1" || want.AcceptedWindowMinutes != 10 || !want.ObservedAt.After(acceptedAt) ||
		want.Queued != 1 || want.Running != 4 || want.RunningWithCurrentLease != 2 || want.RunningWithoutUsableLease != 2 ||
		want.ConfiguredEnabledNodes != 2 || want.ConfiguredEnabledSlots != 6 || want.RecentlyAccepted != 6 ||
		want.NodesWithCurrentLease != 1 || want.EnabledNodesWithoutCurrentLease != 1 ||
		want.RecentWithSystemError != 2 || want.RecentRecoveryExhausted != 1 ||
		want.OldestQueuedAcceptedAt == nil || !want.OldestQueuedAcceptedAt.Equal(acceptedAt) {
		t.Fatalf("unexpected aggregate snapshot: %+v", want)
	}
	raw, err := json.Marshal(want)
	must(t, err)
	for _, private := range []string{subject, nodeID, disabledID, idleID, current.Token, stale.Token, oldest.ID, f.request.Source} {
		if strings.Contains(string(raw), private) {
			t.Fatal("snapshot exposed a private fixture value")
		}
	}
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(f.ctx, "UPDATE judge_tasks SET recovery_generation=recovery_generation+1 WHERE submission_id IN ($1,$2)", healthy.ID, second.ID)
	must(t, err)
	if got := read(); got.RunningWithCurrentLease != 2 || got.NodesWithCurrentLease != 1 {
		t.Fatal("observation waited for or saw an uncommitted row lock")
	}
	must(t, tx.Commit(f.ctx))
	if got := read(); got.RunningWithCurrentLease != 0 || got.RunningWithoutUsableLease != 4 ||
		got.NodesWithCurrentLease != 0 || got.EnabledNodesWithoutCurrentLease != 2 {
		t.Fatal("lease from the previous recovery generation still counted")
	}
	_, err = f.a.Exec(f.ctx, "UPDATE judge_tasks SET recovery_generation=1 WHERE submission_id IN ($1,$2)", healthy.ID, second.ID)
	must(t, err)
	if got := read(); got.RunningWithCurrentLease != 2 || got.NodesWithCurrentLease != 1 {
		t.Fatal("fixture did not restore the current leases before rotating credentials")
	}
	_, err = f.a.Exec(f.ctx, "UPDATE worker_nodes SET credential_version=2 WHERE id=$1", nodeID)
	must(t, err)
	_, err = f.a.Exec(f.ctx, "UPDATE judge_tasks SET judge_generation=judge_generation+1 WHERE submission_id=$1", finished.ID)
	must(t, err)
	if got := read(); got.RunningWithCurrentLease != 0 || got.RecentWithSystemError != 1 ||
		got.NodesWithCurrentLease != 0 || got.EnabledNodesWithoutCurrentLease != 2 {
		t.Fatal("rotated credentials or previous-judge-generation SE still counted")
	}
}

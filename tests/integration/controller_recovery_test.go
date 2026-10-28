//go:build integration

package integration_test

import (
	"context"
	"crypto/x509"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Seed while controllers are offline, then observe recovery without calling
// Store.Recover. These are real processes/database operations, not executions.
func testControllerRecovery(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *postgres.Store) {
	binary := filepath.Join(t.TempDir(), "worker-api")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/worker-api")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build recovery controller: %v\n%s", err, output)
	}
	node := submission.NodeIdentity{ID: "recovery-backlog-node", CredentialVersion: 1}
	_, err := pool.Exec(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)`, node.ID)
	must(t, err)
	var expired []submission.Lease
	var live submission.Lease
	for i := range 66 {
		row, _, err := store.Accept(ctx, "recovery-fixture", "m0-training", fmt.Sprintf("recovery-backlog-%04d", i), submission.Request{
			MaterialVersion: experiment.Version, Language: "gnu++17", Source: "// synthetic recovery fixture; never executed\n",
		})
		must(t, err)
		lease, err := store.Claim(ctx, node)
		must(t, err)
		if lease.SubmissionID != row.ID {
			t.Fatal("unexpected queued fixture before controller startup")
		}
		if i == 65 {
			live = lease
			continue
		}
		_, err = pool.Exec(ctx, `UPDATE judge_attempts SET lease_until=clock_timestamp()-interval '1 second' WHERE token=$1`, lease.Token)
		must(t, err)
		expired = append(expired, lease)
	}
	ca := testpki.New(t)
	_, certFile, keyFile := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	pair, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/"+node.ID+"/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	client := workerClient(t, ca.Roots, pair)
	for range 2 {
		startWorkerController(t, binary, ca.CAFile, certFile, keyFile, client)
	}
	for _, lease := range expired {
		waitTaskState(t, ctx, pool, lease.SubmissionID, "queued")
	}
	var recovered int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM judge_attempts WHERE node_id=$1 AND state='expired'`, node.ID).Scan(&recovered))
	if recovered != len(expired) {
		t.Fatalf("expired history=%d want=%d", recovered, len(expired))
	}
	row, err := store.Get(ctx, "recovery-fixture", "m0-training", live.SubmissionID)
	must(t, err)
	if row.State != "running" {
		t.Fatal("scanner recovered a still-valid lease")
	}
	result := submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("a", 64)}
	must(t, store.Report(ctx, node, live.Token, result))
	seen := make(map[string]bool)
	for range expired {
		lease, err := store.Claim(ctx, node)
		must(t, err)
		if seen[lease.SubmissionID] || lease.AttemptNumber != 2 {
			t.Fatal("concurrent scanners duplicated a recovery or changed its attempt number")
		}
		seen[lease.SubmissionID] = true
		must(t, store.Report(ctx, node, lease.Token, result))
	}
	for _, lease := range expired {
		if !seen[lease.SubmissionID] {
			t.Fatal("backlog submission was lost")
		}
	}
}

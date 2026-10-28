//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testAcquisition(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ca *testpki.Authority, bases []string, root string) {
	t.Helper()
	version, manifest, err := experiment.SelectFixture(experiment.PinnedVersion)
	must(t, err)
	store, err := postgres.New(pool, version, manifest)
	must(t, err)
	nodes := []string{"acquisition-a", "acquisition-b"}
	clients := make([][]*workerclient.Acquirer, 2)
	reporters := make([]*workerclient.Reporter, 2)
	for i, node := range nodes {
		_, err := pool.Exec(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)`, node)
		must(t, err)
		cert, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/"+node+"/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
		for _, base := range bases {
			a, err := workerclient.NewAcquirer(base, ca.Roots, cert)
			must(t, err)
			t.Cleanup(a.Close)
			clients[i] = append(clients[i], a)
		}
		reporters[i], err = workerclient.NewReporter(bases[1], ca.Roots, cert)
		must(t, err)
		t.Cleanup(reporters[i].Close)
	}
	source := "// transport fixture only; 原样源码😀\n"
	row, _, err := store.Accept(ctx, "acquisition", "m0-training", "acquisition-client-01", submission.Request{MaterialVersion: version, Language: "gnu++17", Source: source})
	must(t, err)
	type claimed struct {
		node  int
		lease submission.Lease
		err   error
	}
	results := make(chan claimed, 2)
	for i := range 2 {
		go func() { l, err := clients[i][i].Claim(ctx); results <- claimed{i, l, err} }()
	}
	winner := -1
	var lease submission.Lease
	for range 2 {
		r := <-results
		if r.err == nil {
			if winner >= 0 || r.lease.SubmissionID != row.ID {
				t.Fatal("duplicate or wrong claim")
			}
			winner, lease = r.node, r.lease
		} else if !errors.Is(r.err, submission.ErrNoTask) {
			t.Fatal(r.err)
		}
	}
	if winner < 0 {
		t.Fatal("task not claimed")
	}
	for _, client := range clients[winner] {
		d, blobs, err := client.Fetch(ctx, lease)
		must(t, err)
		if d.Source != source || !bytes.Equal(d.Manifest, manifest) || d.SubmissionID != row.ID || len(blobs) != 4 {
			t.Fatal("assignment not preserved")
		}
		for _, data := range []string{"1 2\n", "3\n", "-7 4\n", "-3\n"} {
			if string(blobs[materialHash([]byte(data))]) != data {
				t.Fatal("material changed")
			}
		}
	}
	deny := func(client *workerclient.Acquirer, l submission.Lease, want error) {
		t.Helper()
		d, blobs, err := client.Fetch(ctx, l)
		if !errors.Is(err, want) || blobs != nil || d.SubmissionID != "" {
			t.Fatalf("unexpected material outcome: %v", err)
		}
	}
	deny(clients[1-winner][0], lease, submission.ErrLease)
	path := filepath.Join(root, materialHash([]byte("-3\n")))
	must(t, os.WriteFile(path, []byte("bad"), 0600))
	t.Cleanup(func() {
		if err := os.WriteFile(path, []byte("-3\n"), 0600); err != nil {
			t.Error(err)
		}
	})
	deny(clients[winner][0], lease, workerclient.ErrMaterials)
	must(t, os.Remove(path))
	deny(clients[winner][1], lease, workerclient.ErrMaterials)
	must(t, experiment.WriteMaterials(root))
	_, _, err = clients[winner][0].Fetch(ctx, lease)
	must(t, err)
	_, err = pool.Exec(ctx, `UPDATE worker_nodes SET enabled=false WHERE id=$1`, nodes[winner])
	must(t, err)
	deny(clients[winner][1], lease, submission.ErrNode)
	_, err = clients[winner][0].Claim(ctx)
	if !errors.Is(err, submission.ErrNode) {
		t.Fatal("revoked node claimed")
	}
	_, err = pool.Exec(ctx, `UPDATE worker_nodes SET enabled=true WHERE id=$1`, nodes[winner])
	must(t, err)
	_, err = pool.Exec(ctx, `UPDATE judge_attempts SET lease_until=clock_timestamp()-interval '1 second' WHERE token=$1`, lease.Token)
	must(t, err)
	deny(clients[winner][0], lease, submission.ErrLease)
	waitTaskState(t, ctx, pool, lease.SubmissionID, "queued")
	current, err := clients[winner][1].Claim(ctx)
	must(t, err)
	if current.SubmissionID != lease.SubmissionID || current.Token == lease.Token || current.AttemptNumber <= lease.AttemptNumber {
		t.Fatal("recovery attribution changed")
	}
	deny(clients[winner][1], lease, submission.ErrLease)
	_, _, err = clients[winner][0].Fetch(ctx, current)
	must(t, err)
	must(t, reporters[winner].Report(ctx, current.Token, submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("a", 64)})) // synthetic protocol receipt
	deny(clients[winner][0], current, submission.ErrLease)
	_, err = clients[winner][1].Claim(ctx)
	if !errors.Is(err, submission.ErrNoTask) {
		t.Fatal("finished task claimed again")
	}
}

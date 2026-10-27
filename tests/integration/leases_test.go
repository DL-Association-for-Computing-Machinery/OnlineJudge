//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Synthetic result reports test persistence only. No contestant program runs here.
func testLeases(t *testing.T, ctx context.Context, pool *pgxpool.Pool, first, second *postgres.Store) {
	nodes := []submission.NodeIdentity{{ID: "test-node-a", CredentialVersion: 1}, {ID: "test-node-b", CredentialVersion: 1}}
	for _, n := range nodes {
		_, err := pool.Exec(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,$2)`, n.ID, n.CredentialVersion)
		must(t, err)
	}
	ac := submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("a", 64)}
	for range 16 {
		lease, err := first.Claim(ctx, nodes[0])
		if errors.Is(err, submission.ErrNoTask) {
			break
		}
		must(t, err)
		must(t, first.Report(ctx, nodes[0], lease.Token, ac))
	}
	sequence := 0
	accept := func() submission.Receipt {
		sequence++
		row, _, err := first.Accept(ctx, "lease-fixture", "m0-training", fmt.Sprintf("lease-fixture-%04d", sequence), submission.Request{
			MaterialVersion: experiment.Version, Language: "gnu++17", Source: "// synthetic lease test; not executed\n",
		})
		must(t, err)
		return row
	}
	expire := func(token string) {
		_, err := pool.Exec(ctx, `UPDATE judge_attempts SET lease_until=clock_timestamp()-interval '1 second' WHERE token=$1`, token)
		must(t, err)
	}
	t.Run("two nodes compete without duplicate claims", func(t *testing.T) {
		expected := accept()
		type answer struct {
			node  submission.NodeIdentity
			lease submission.Lease
			err   error
		}
		results := make(chan answer, 2)
		var wg sync.WaitGroup
		for i, store := range []*postgres.Store{first, second} {
			wg.Add(1)
			go func(i int, store *postgres.Store) {
				defer wg.Done()
				lease, err := store.Claim(ctx, nodes[i])
				results <- answer{nodes[i], lease, err}
			}(i, store)
		}
		wg.Wait()
		close(results)
		successes := 0
		for result := range results {
			if errors.Is(result.err, submission.ErrNoTask) {
				continue
			}
			must(t, result.err)
			successes++
			if result.lease.SubmissionID != expected.ID {
				t.Fatal("wrong task claimed")
			}
			if _, err := first.Claim(ctx, result.node); !errors.Is(err, submission.ErrNoTask) {
				t.Fatal("node over-prefetched")
			}
			until, err := second.Renew(ctx, result.node, result.lease.Token)
			must(t, err)
			if until.Before(result.lease.Until) {
				t.Fatal("renewal shortened lease")
			}
			must(t, first.Report(ctx, result.node, result.lease.Token, ac))
			must(t, second.Report(ctx, result.node, result.lease.Token, ac))
			changed := ac
			changed.EvidenceSHA256 = strings.Repeat("b", 64)
			if !errors.Is(second.Report(ctx, result.node, result.lease.Token, changed), submission.ErrResultConflict) {
				t.Fatal("conflicting result accepted")
			}
		}
		if successes != 1 {
			t.Fatalf("claims=%d", successes)
		}
		row, err := first.Get(ctx, "lease-fixture", "m0-training", expected.ID)
		must(t, err)
		if row.State != "finished" || row.Verdict == nil || *row.Verdict != "AC" {
			t.Fatal("synthetic result not committed")
		}
	})
	t.Run("expiry rejects late renew and report before recovery", func(t *testing.T) {
		expected := accept()
		old, err := first.Claim(ctx, nodes[0])
		must(t, err)
		expire(old.Token)
		if _, err := second.Renew(ctx, nodes[0], old.Token); !errors.Is(err, submission.ErrLease) {
			t.Fatal("expired renew accepted")
		}
		if !errors.Is(second.Report(ctx, nodes[0], old.Token, ac), submission.ErrLease) {
			t.Fatal("expired report accepted")
		}
		count, err := second.Recover(ctx)
		must(t, err)
		if count != 1 {
			t.Fatalf("recover=%d", count)
		}
		current, err := second.Claim(ctx, nodes[1])
		must(t, err)
		if current.SubmissionID != expected.ID || current.AttemptNumber != old.AttemptNumber+1 || current.JudgeGeneration != old.JudgeGeneration || current.RecoveryGeneration != old.RecoveryGeneration {
			t.Fatal("counters conflated")
		}
		if !errors.Is(first.Report(ctx, nodes[0], old.Token, ac), submission.ErrLease) {
			t.Fatal("old attempt overwrote new attempt")
		}
		must(t, second.Report(ctx, nodes[1], current.Token, ac))
	})
	t.Run("revocation and credential rotation fail closed", func(t *testing.T) {
		accept()
		lease, err := first.Claim(ctx, nodes[0])
		must(t, err)
		if _, err := second.Renew(ctx, nodes[1], lease.Token); !errors.Is(err, submission.ErrLease) {
			t.Fatal("foreign node renewed")
		}
		_, err = pool.Exec(ctx, `UPDATE worker_nodes SET enabled=false,credential_version=2 WHERE id=$1`, nodes[0].ID)
		must(t, err)
		if !errors.Is(first.Report(ctx, nodes[0], lease.Token, ac), submission.ErrNode) {
			t.Fatal("revoked node reported")
		}
		if _, err := first.Claim(ctx, nodes[0]); !errors.Is(err, submission.ErrNode) {
			t.Fatal("revoked node claimed")
		}
		count, err := second.Recover(ctx)
		must(t, err)
		if count != 1 {
			t.Fatal("revoked attempt not recovered")
		}
		_, err = pool.Exec(ctx, `UPDATE worker_nodes SET enabled=true WHERE id=$1`, nodes[0].ID)
		must(t, err)
		if _, err := first.Claim(ctx, nodes[0]); !errors.Is(err, submission.ErrNode) {
			t.Fatal("old credential revived")
		}
		nodes[0].CredentialVersion = 2
		next, err := first.Claim(ctx, nodes[0])
		must(t, err)
		must(t, first.Report(ctx, nodes[0], next.Token, ac))
	})
	t.Run("system error has bounded retries and historical acknowledgment", func(t *testing.T) {
		expected := accept()
		se := ac
		se.Verdict = "SE"
		var old submission.Lease
		for i := 1; i <= 3; i++ {
			lease, err := first.Claim(ctx, nodes[0])
			must(t, err)
			if lease.SubmissionID != expected.ID || lease.AttemptNumber != int32(i) {
				t.Fatal("unexpected retry")
			}
			if i > 1 {
				must(t, second.Report(ctx, nodes[0], old.Token, se))
			}
			must(t, first.Report(ctx, nodes[0], lease.Token, se))
			old = lease
		}
		row, err := second.Get(ctx, "lease-fixture", "m0-training", expected.ID)
		must(t, err)
		if row.State != "failed" || row.Verdict != nil {
			t.Fatal("system error became contestant verdict")
		}
		if _, err := second.Claim(ctx, nodes[0]); !errors.Is(err, submission.ErrNoTask) {
			t.Fatal("unbounded retries")
		}
	})
	t.Run("repeated node loss exhausts recovery budget", func(t *testing.T) {
		expected := accept()
		for range 3 {
			lease, err := first.Claim(ctx, nodes[1])
			must(t, err)
			expire(lease.Token)
			_, err = second.Recover(ctx)
			must(t, err)
		}
		row, err := first.Get(ctx, "lease-fixture", "m0-training", expected.ID)
		must(t, err)
		if row.State != "failed" || row.Verdict != nil {
			t.Fatal("node loss concealed")
		}
	})
	t.Run("restore and rejudge generations fence stale reports", func(t *testing.T) {
		for _, query := range []string{
			`UPDATE judge_tasks SET recovery_generation=recovery_generation+1 WHERE submission_id=$1`,
			`UPDATE judge_tasks SET judge_generation=judge_generation+1 WHERE submission_id=$1`,
		} {
			expected := accept()
			lease, err := first.Claim(ctx, nodes[0])
			must(t, err)
			_, err = pool.Exec(ctx, query, expected.ID)
			must(t, err)
			if !errors.Is(second.Report(ctx, nodes[0], lease.Token, ac), submission.ErrLease) {
				t.Fatal("old generation accepted")
			}
			if _, err := second.Renew(ctx, nodes[0], lease.Token); !errors.Is(err, submission.ErrLease) {
				t.Fatal("old generation renewed")
			}
			expire(lease.Token)
		}
	})
}

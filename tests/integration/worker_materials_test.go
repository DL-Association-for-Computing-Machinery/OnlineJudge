//go:build integration

package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func materialBody(token string) string {
	return `{"lease_token":"` + token + `","test_id":"01","kind":"input"}`
}
func materialHash(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

func testWorkerMaterials(t *testing.T, owner, other *http.Client, urls []string, root, id, token string) {
	t.Helper()
	call := func(c *http.Client, base, action, body string, want int) []byte {
		t.Helper()
		status, data, _, err := workerRequest(c, base, action, body)
		must(t, err)
		if status != want {
			t.Fatalf("%s status=%d want=%d body=%s", action, status, want, data)
		}
		return data
	}
	for _, base := range urls {
		var descriptor materials.Descriptor
		must(t, json.Unmarshal(call(owner, base, "assignment", `{"lease_token":"`+token+`"}`, 200), &descriptor))
		if descriptor.SubmissionID != id || descriptor.Source != "// synthetic protocol fixture; 原样源码😀; never executed\n" || descriptor.SourceSHA256 != materialHash([]byte(descriptor.Source)) || descriptor.ManifestSHA256 != materialHash(descriptor.Manifest) || string(descriptor.Manifest) != string(experiment.Manifest()) {
			t.Fatal("source/manifest bytes or digests changed")
		}
		for i, values := range [][]string{{"1 2\n", "3\n"}, {"-7 4\n", "-3\n"}} {
			for j, kind := range []string{"input", "answer"} {
				body := fmt.Sprintf(`{"lease_token":%q,"test_id":"%02d","kind":%q}`, token, i+1, kind)
				req, err := http.NewRequest("POST", base+"/api/v1/worker/material", strings.NewReader(body))
				must(t, err)
				req.Header.Set("Content-Type", "application/json")
				resp, err := owner.Do(req)
				must(t, err)
				data, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				must(t, err)
				if resp.StatusCode != 200 || string(data) != values[j] || resp.Header.Get("X-Content-SHA256") != materialHash(data) || resp.ContentLength != int64(len(data)) || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Content-Type") != "application/octet-stream" {
					t.Fatalf("invalid material response %d %q", resp.StatusCode, data)
				}
			}
		}
		call(other, base, "assignment", `{"lease_token":"`+token+`"}`, 409)
		call(other, base, "material", materialBody(token), 409)
		call(owner, base, "material", materialBody(strings.Repeat("f", 64)), 409)
		call(owner, base, "material", strings.Replace(materialBody(token), `"01"`, `"99"`, 1), 404)
		call(owner, base, "material", strings.Replace(materialBody(token), `"01"`, `"../01"`, 1), 400)
		call(owner, base, "material", strings.Replace(materialBody(token), `"input"`, `"source"`, 1), 400)
		call(owner, base, "assignment", `{"lease_token":"`+token+`","node_id":"other"}`, 400)
	}
	path := filepath.Join(root, materialHash([]byte("1 2\n")))
	must(t, os.WriteFile(path, []byte("corrupt"), 0600))
	t.Cleanup(func() {
		if err := os.WriteFile(path, []byte("1 2\n"), 0600); err != nil {
			t.Error(err)
		}
	})
	call(owner, urls[0], "material", materialBody(token), 503)
	must(t, os.Remove(path))
	call(owner, urls[1], "material", materialBody(token), 503)
	must(t, experiment.WriteMaterials(root))
	call(owner, urls[0], "material", materialBody(token), 200)
}

func testMaterialLocks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *postgres.Store, node submission.NodeIdentity) {
	newLease := func(key string) submission.Lease {
		_, _, err := store.Accept(ctx, "material-fixture", "m0-training", key, submission.Request{MaterialVersion: experiment.Version, Language: "gnu++17", Source: "// lock fixture\n"})
		must(t, err)
		lease, err := store.Claim(ctx, node)
		must(t, err)
		return lease
	}
	for _, lock := range []string{"node", "attempt"} {
		t.Run("expiry after "+lock+" lock wait", func(t *testing.T) {
			lease := newLease("material-lock-" + lock)
			tx, err := pool.Begin(ctx)
			must(t, err)
			defer tx.Rollback(ctx)
			var pid uint32
			must(t, tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
			if lock == "node" {
				_, err = tx.Exec(ctx, `SELECT id FROM worker_nodes WHERE id=$1 FOR UPDATE`, node.ID)
			} else {
				_, err = tx.Exec(ctx, `SELECT token FROM judge_attempts WHERE token=$1 FOR UPDATE`, lease.Token)
			}
			must(t, err)
			done := make(chan error, 1)
			go func() { _, err := store.Materials(ctx, node, lease.Token); done <- err }()
			// Observe an actual blocked query, rather than assuming scheduling from sleep.
			blocked := false
			for range 100 {
				must(t, pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)))`, pid).Scan(&blocked))
				if blocked {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !blocked {
				t.Fatal("material authorization did not wait for row lock")
			}
			_, err = tx.Exec(ctx, `UPDATE judge_attempts SET lease_until=clock_timestamp()-interval '1 second' WHERE token=$1`, lease.Token)
			must(t, err)
			must(t, tx.Commit(ctx))
			if err := <-done; !errors.Is(err, submission.ErrLease) {
				t.Fatalf("expired while blocked = %v", err)
			}
			_, err = store.Recover(ctx)
			must(t, err)
			current, err := store.Claim(ctx, node)
			must(t, err)
			must(t, store.Report(ctx, node, current.Token, submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("a", 64)}))
		})
	}
	for _, generation := range []string{"judge_generation", "recovery_generation"} {
		t.Run(generation, func(t *testing.T) {
			lease := newLease("material-" + generation)
			query := `UPDATE judge_tasks SET judge_generation=judge_generation+1 WHERE submission_id=$1`
			if generation == "recovery_generation" {
				query = `UPDATE judge_tasks SET recovery_generation=recovery_generation+1 WHERE submission_id=$1`
			}
			_, err := pool.Exec(ctx, query, lease.SubmissionID)
			must(t, err)
			if _, err := store.Materials(ctx, node, lease.Token); !errors.Is(err, submission.ErrLease) {
				t.Fatalf("old generation = %v", err)
			}
			// Restore only this test's mutation, then complete its synthetic lease.
			_, err = pool.Exec(ctx, `UPDATE judge_tasks SET judge_generation=$2,recovery_generation=$3 WHERE submission_id=$1`, lease.SubmissionID, lease.JudgeGeneration, lease.RecoveryGeneration)
			must(t, err)
			must(t, store.Report(ctx, node, lease.Token, submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("a", 64)}))
		})
	}
	t.Run("corrupt registered manifest", func(t *testing.T) {
		lease := newLease("material-corrupt-manifest")
		_, err := pool.Exec(ctx, `UPDATE submission_materials SET manifest='{}'::jsonb WHERE version_id=$1`, experiment.Version)
		must(t, err)
		_, readErr := store.Materials(ctx, node, lease.Token)
		_, err = pool.Exec(ctx, `UPDATE submission_materials SET manifest=$2::jsonb WHERE version_id=$1`, experiment.Version, experiment.Manifest())
		must(t, err)
		if !errors.Is(readErr, submission.ErrMaterial) {
			t.Fatalf("corrupt manifest = %v", readErr)
		}
		must(t, store.Report(ctx, node, lease.Token, submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("a", 64)}))
	})
}

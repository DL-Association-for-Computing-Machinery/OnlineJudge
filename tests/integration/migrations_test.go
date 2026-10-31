//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// Each writer follows the source/task transaction boundary while Down races it.
// Observing the lock wait makes the race deterministic without timing the commit.
func testConcurrentRollback(t *testing.T, pool *pgxpool.Pool, provider *goose.Provider) {
	for _, version := range []int{2, 1} {
		t.Run(fmt.Sprintf("version_%d", version), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := provider.DownTo(ctx, int64(version))
			must(t, err)
			_, err = pool.Exec(ctx, `INSERT INTO submission_materials VALUES ('rollback-fixture',repeat('a',64),'{}')`)
			must(t, err)
			writer, err := pool.Begin(ctx)
			must(t, err)
			defer writer.Rollback(context.Background())
			_, err = writer.Exec(ctx, `INSERT INTO submissions
				(id,subject_id,context_id,idempotency_key,request_sha256,material_version,manifest_sha256,language,source_code)
				VALUES ('rollback-race','synthetic','training','rollback-race-key-01',repeat('b',64),'rollback-fixture',repeat('a',64),'gnu++17','int main(){}');
				INSERT INTO judge_tasks(submission_id) VALUES ('rollback-race')`)
			must(t, err)
			writerPID := writer.Conn().PgConn().PID()
			result := make(chan error, 1)
			go func() { _, err := provider.Down(ctx); result <- err }()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				var waiting bool
				must(t, pool.QueryRow(ctx, `SELECT EXISTS (
					SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)))`, writerPID).Scan(&waiting))
				if waiting {
					break
				}
				select {
				case err := <-result:
					t.Fatalf("migration did not wait for the in-flight writer: %v", err)
				case <-ctx.Done():
					t.Fatal("migration never blocked on the writer")
				case <-ticker.C:
				}
			}
			must(t, writer.Commit(ctx))
			if err := <-result; err == nil {
				t.Error("rollback succeeded after a concurrent submission committed")
			}
			var currentVersion int
			must(t, pool.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version`).Scan(&currentVersion))
			if currentVersion != version {
				t.Errorf("schema changed: version=%d want=%d", currentVersion, version)
			}
			var sourceExists bool
			must(t, pool.QueryRow(ctx, `SELECT to_regclass('submissions') IS NOT NULL`).Scan(&sourceExists))
			if !sourceExists {
				t.Error("accepted source table was dropped")
			} else {
				var source, state string
				must(t, pool.QueryRow(ctx, `SELECT source_code,state FROM submissions JOIN judge_tasks ON id=submission_id WHERE id='rollback-race'`).Scan(&source, &state))
				if source != "int main(){}" || state != "queued" {
					t.Errorf("accepted data changed: source=%q state=%q", source, state)
				}
				_, err = pool.Exec(ctx, `DELETE FROM judge_tasks; DELETE FROM submissions; DELETE FROM submission_materials`)
				must(t, err)
			}
			// Restore an empty current schema for the next case and acceptance tests.
			_, err = provider.Up(ctx)
			must(t, err)
		})
	}
}

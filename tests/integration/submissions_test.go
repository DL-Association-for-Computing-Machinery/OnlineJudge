//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

const token = "synthetic-test-only-token-never-a-real-credential"

func TestAcceptance(t *testing.T) {
	dsn := os.Getenv("OJ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("OJ_TEST_DATABASE_URL must point to a disposable test database; use make db-test")
	}
	ctx := context.Background()
	conn, err := sql.Open("pgx", dsn)
	must(t, err)
	defer conn.Close()
	conn.SetMaxOpenConns(4)
	// Guard against accidentally running this destructive fixture suite on application data.
	var count int
	must(t, conn.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&count))
	if count != 0 {
		t.Fatal("integration database must be empty")
	}
	provider, err := db.Provider(conn)
	must(t, err)
	pool, err := postgres.Open(ctx, dsn)
	must(t, err)
	defer pool.Close()

	t.Run("uninitialized schema rejected", func(t *testing.T) {
		if db.CheckSchema(ctx, pool) == nil {
			t.Fatal("uninitialized schema accepted")
		}
	})
	t.Run("concurrent migration and repeat", func(t *testing.T) {
		other, err := sql.Open("pgx", dsn)
		must(t, err)
		defer other.Close()
		second, err := db.Provider(other)
		must(t, err)
		errs := make(chan error, 2)
		for _, p := range []*goose.Provider{provider, second} {
			go func(p *goose.Provider) { _, err := p.Up(ctx); errs <- err }(p)
		}
		must(t, <-errs)
		must(t, <-errs)
		result, err := provider.Up(ctx)
		must(t, err)
		if len(result) != 0 {
			t.Fatal("repeat migration changed schema")
		}
		must(t, db.CheckSchema(ctx, pool))
	})
	t.Run("migration lock timeout and recovery", func(t *testing.T) {
		held, err := pool.Acquire(ctx)
		must(t, err)
		defer held.Release()
		_, err = held.Exec(ctx, `SELECT pg_advisory_lock($1)`, db.MigrationLockID)
		must(t, err)
		defer func() { _, _ = held.Exec(ctx, `SELECT pg_advisory_unlock($1)`, db.MigrationLockID) }()
		short, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
		defer cancel()
		// Up with no pending migrations is read-only and deliberately avoids a
		// lock. Down exercises an actual schema-changing operation.
		_, err = provider.Down(short)
		if err == nil {
			t.Fatal("migration ignored lock")
		}
		_, err = held.Exec(ctx, `SELECT pg_advisory_unlock($1)`, db.MigrationLockID)
		must(t, err)
		_, err = provider.Up(ctx)
		must(t, err)
	})
	t.Run("empty rollback and upgrade", func(t *testing.T) {
		_, err := provider.DownTo(ctx, 0)
		must(t, err)
		if db.CheckSchema(ctx, pool) == nil {
			t.Fatal("rolled back schema accepted")
		}
		_, err = provider.Up(ctx)
		must(t, err)
	})

	store, err := postgres.New(pool, experiment.Version, experiment.Manifest())
	must(t, err)
	request := submission.Request{MaterialVersion: experiment.Version, Language: "gnu++17", Source: "int main() { return 0; }\n"}
	t.Run("missing and conflicting material", func(t *testing.T) {
		_, _, err := store.Accept(ctx, "subject", "training", "missing-material-01", request)
		if !errors.Is(err, submission.ErrMaterial) {
			t.Fatalf("unexpected error: %v", err)
		}
		must(t, store.RegisterFixture(ctx))
		must(t, store.RegisterFixture(ctx))
		conflicting, err := postgres.New(pool, experiment.Version, []byte(`{"different":true}`))
		must(t, err)
		if !errors.Is(conflicting.RegisterFixture(ctx), submission.ErrMaterial) {
			t.Fatal("material overwritten")
		}
	})

	secondPool, err := postgres.Open(ctx, dsn)
	must(t, err)
	defer secondPool.Close()
	secondStore, err := postgres.New(secondPool, experiment.Version, experiment.Manifest())
	must(t, err)
	servers := []*httptest.Server{
		httptest.NewServer(httpapi.NewHandler(httpapi.Experiment{Store: store, Token: token})),
		httptest.NewServer(httpapi.NewHandler(httpapi.Experiment{Store: secondStore, Token: token})),
	}
	defer servers[0].Close()
	defer servers[1].Close()
	data, err := json.Marshal(request)
	must(t, err)
	var accepted submission.Receipt
	t.Run("two API concurrent identical submissions", func(t *testing.T) {
		type response struct {
			status int
			body   []byte
			err    error
		}
		results := make(chan response, 24)
		var wg sync.WaitGroup
		for i := range 24 {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				status, body, err := call(servers[i%2].URL, "POST", "/submissions", "concurrent-key-0001", data, token)
				results <- response{status, body, err}
			}(i)
		}
		wg.Wait()
		close(results)
		created := 0
		for result := range results {
			must(t, result.err)
			if result.status != 200 && result.status != 202 {
				t.Fatalf("status %d: %s", result.status, result.body)
			}
			if result.status == 202 {
				created++
			}
			var value submission.Receipt
			must(t, json.Unmarshal(result.body, &value))
			if accepted.ID == "" {
				accepted = value
			}
			if value.ID != accepted.ID || value.State != "queued" || value.Verdict != nil {
				t.Fatalf("invalid receipt: %+v", value)
			}
		}
		if created != 1 {
			t.Fatalf("created %d submissions", created)
		}
		var submissions, tasks int
		must(t, conn.QueryRow(`SELECT count(*) FROM submissions`).Scan(&submissions))
		must(t, conn.QueryRow(`SELECT count(*) FROM judge_tasks`).Scan(&tasks))
		if submissions != 1 || tasks != 1 {
			t.Fatalf("submissions=%d tasks=%d", submissions, tasks)
		}
	})
	t.Run("same key different content conflicts", func(t *testing.T) {
		changed := request
		changed.Source += "// changed"
		body, _ := json.Marshal(changed)
		status, _, err := call(servers[1].URL, "POST", "/submissions", "concurrent-key-0001", body, token)
		must(t, err)
		if status != 409 {
			t.Fatalf("status %d", status)
		}
	})
	t.Run("unknown acknowledgment recovered by original key", func(t *testing.T) {
		handler := httpapi.NewHandler(httpapi.Experiment{Store: store, Token: token})
		lost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, r)
			socket, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = socket.Close()
			}
		}))
		defer lost.Close()
		_, _, err := call(lost.URL, "POST", "/submissions", "lost-response-0001", data, token)
		if err == nil {
			t.Fatal("expected lost response")
		}
		status, body, err := call(servers[1].URL, "GET", "/submission-receipt", "lost-response-0001", nil, token)
		must(t, err)
		if status != 200 {
			t.Fatalf("receipt %d: %s", status, body)
		}
		status, _, err = call(servers[1].URL, "POST", "/submissions", "lost-response-0001", data, token)
		must(t, err)
		if status != 200 {
			t.Fatalf("retry created another submission: %d", status)
		}
	})
	t.Run("ownership and context isolation", func(t *testing.T) {
		for _, identity := range [][2]string{{"another-subject", "m0-training"}, {"m0-operator", "other-context"}} {
			_, err := store.Get(ctx, identity[0], identity[1], accepted.ID)
			if !errors.Is(err, submission.ErrNotFound) {
				t.Fatalf("ownership error: %v", err)
			}
			_, err = store.Receipt(ctx, identity[0], identity[1], "concurrent-key-0001")
			if !errors.Is(err, submission.ErrNotFound) {
				t.Fatalf("receipt ownership error: %v", err)
			}
		}
	})
	t.Run("database failure rolls back both records", func(t *testing.T) {
		_, err := conn.Exec(`CREATE FUNCTION reject_task() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected task failure'; END $$`)
		must(t, err)
		_, err = conn.Exec(`CREATE TRIGGER reject_task BEFORE INSERT ON judge_tasks FOR EACH ROW EXECUTE FUNCTION reject_task()`)
		must(t, err)
		_, _, err = store.Accept(ctx, "subject", "training", "transaction-fail-01", request)
		if err == nil {
			t.Fatal("injected failure accepted")
		}
		var count int
		must(t, conn.QueryRow(`SELECT count(*) FROM submissions WHERE idempotency_key=$1`, "transaction-fail-01").Scan(&count))
		if count != 0 {
			t.Fatal("half a submission persisted")
		}
		_, err = conn.Exec(`DROP TRIGGER reject_task ON judge_tasks; DROP FUNCTION reject_task()`)
		must(t, err)
		_, created, err := store.Accept(ctx, "subject", "training", "transaction-fail-01", request)
		must(t, err)
		if !created {
			t.Fatal("failed transaction consumed key")
		}
	})
	t.Run("restart preserves source and receipt", func(t *testing.T) {
		reopened, err := postgres.Open(ctx, dsn)
		must(t, err)
		defer reopened.Close()
		fresh, err := postgres.New(reopened, experiment.Version, experiment.Manifest())
		must(t, err)
		result, err := fresh.Get(ctx, "m0-operator", "m0-training", accepted.ID)
		must(t, err)
		if result.ID != accepted.ID {
			t.Fatal("receipt lost")
		}
		var source string
		must(t, conn.QueryRow(`SELECT source_code FROM submissions WHERE id=$1`, accepted.ID).Scan(&source))
		if source != request.Source {
			t.Fatal("source changed")
		}
	})
	t.Run("rollback refuses accepted data atomically", func(t *testing.T) {
		if _, err := provider.Down(ctx); err == nil {
			t.Fatal("destructive rollback allowed")
		}
		must(t, db.CheckSchema(ctx, pool))
		_, err := store.Get(ctx, "m0-operator", "m0-training", accepted.ID)
		must(t, err)
	})
	t.Run("corrupt manifest blocks new acceptance", func(t *testing.T) {
		_, err := conn.Exec(`UPDATE submission_materials SET manifest='{}'::jsonb`)
		must(t, err)
		_, _, err = store.Accept(ctx, "subject", "training", "corrupt-material-01", request)
		if !errors.Is(err, submission.ErrMaterial) {
			t.Fatalf("corrupt material accepted: %v", err)
		}
		_, _, err = store.Accept(ctx, "m0-operator", "m0-training", "concurrent-key-0001", request)
		must(t, err)
		_, err = conn.Exec(`UPDATE submission_materials SET manifest=$1`, experiment.Manifest())
		must(t, err)
	})
	t.Run("authentication input and response boundaries", func(t *testing.T) {
		for _, tc := range []struct {
			name, key, auth string
			body            []byte
			status          int
		}{
			{"unauthenticated", "authentication-01", "", data, 401},
			{"wrong token", "authentication-01", "wrong", data, 401},
			{"short key", "short", token, data, 400},
			{"unknown field", "unknown-field-01", token, []byte(`{"admin":true}`), 400},
			{"trailing JSON", "trailing-json-01", token, append(append([]byte{}, data...), []byte(` {}`)...), 400},
			{"body budget", "body-over-limit-01", token, []byte(strings.Repeat(" ", 6*submission.MaxSourceBytes+1025)), 413},
		} {
			t.Run(tc.name, func(t *testing.T) {
				status, body, err := call(servers[0].URL, "POST", "/submissions", tc.key, tc.body, tc.auth)
				must(t, err)
				if status != tc.status {
					t.Fatalf("%d: %s", status, body)
				}
			})
		}
		status, body, err := call(servers[1].URL, "GET", "/submissions/"+accepted.ID, "", nil, token)
		must(t, err)
		if status != 200 || bytes.Contains(body, []byte("source")) || bytes.Contains(body, []byte("manifest_sha256")) {
			t.Fatalf("receipt leaks: %s", body)
		}
	})
	t.Run("indexed receipt lookup plan", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		must(t, err)
		defer tx.Rollback(ctx)
		// A tiny fixture naturally uses a seq scan. Explicitly inspect the
		// eligible index plan; this is not production latency/load evidence.
		_, err = tx.Exec(ctx, `SET LOCAL enable_seqscan=off`)
		must(t, err)
		rows, err := tx.Query(ctx, `EXPLAIN (ANALYZE, BUFFERS) SELECT s.id,t.state FROM submissions s JOIN judge_tasks t ON t.submission_id=s.id WHERE subject_id=$1 AND context_id=$2 AND idempotency_key=$3`, "m0-operator", "m0-training", "concurrent-key-0001")
		must(t, err)
		defer rows.Close()
		var plan strings.Builder
		for rows.Next() {
			var line string
			must(t, rows.Scan(&line))
			fmt.Fprintln(&plan, line)
		}
		must(t, rows.Err())
		if !strings.Contains(plan.String(), "Index") {
			t.Fatal("no usable lookup index")
		}
		t.Log(plan.String())
	})
	t.Run("task lease persistence", func(t *testing.T) { testLeases(t, ctx, pool, store, secondStore) })
	t.Run("schema ahead rejected", func(t *testing.T) {
		_, err := conn.Exec(`INSERT INTO goose_db_version(version_id,is_applied) VALUES(3,true)`)
		must(t, err)
		if db.CheckSchema(ctx, pool) == nil {
			t.Fatal("future schema accepted")
		}
	})
	assertPoolBound(t, pool)
	assertPoolBound(t, secondPool)
}

func call(base, method, path, key string, body []byte, auth string) (int, []byte, error) {
	req, err := http.NewRequest(method, base+"/api/v1/experiments"+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+auth)
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	if response.Header.Get("Cache-Control") != "no-store" {
		return 0, nil, errors.New("missing no-store")
	}
	data, err := io.ReadAll(response.Body)
	return response.StatusCode, data, err
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func assertPoolBound(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if pool.Stat().TotalConns() > 4 || pool.Config().MaxConns != 4 {
		t.Fatal("pool exceeded experiment budget")
	}
}

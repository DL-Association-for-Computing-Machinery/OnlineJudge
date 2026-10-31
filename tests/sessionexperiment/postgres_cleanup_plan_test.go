package sessionexperiment

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPostgresCleanupQueryPlan(t *testing.T) {
	f := setupPostgresSessionOptions(t, true, true)
	s := f.stores[0]
	cutoff := postgresCleanupClock(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seedPostgresCleanup(t, s)
	// A small due range amid many future records, plus due rows in another
	// namespace, tests the actual range/ordering path without forcing a plan.
	_, err := s.pool.Exec(ctx, `INSERT INTO session_pg_store_fixture
		SELECT $1,md5(i::text),$2,NULL,$3::bigint+CASE WHEN i<=32 THEN -1000 ELSE 3600000 END,
		$3::bigint+CASE WHEN i<=32 THEN -1000 ELSE 3600000 END
		FROM generate_series(1,4032) i`, s.namespace, digest("fixture-user"), cutoff)
	if err != nil {
		t.Fatal(err)
	}
	other := *s
	other.namespace = digest(t.Name() + "/other")
	seedPostgresCleanup(t, &other)
	_, err = s.pool.Exec(ctx, `INSERT INTO session_pg_store_fixture
		SELECT $1,md5(i::text),$2,NULL,$3::bigint,$3::bigint FROM generate_series(1,2000) i`,
		other.namespace, digest("fixture-user"), cutoff-1000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `ANALYZE session_pg_store_fixture`); err != nil {
		t.Fatal(err)
	}
	otherBefore := postgresRestoreState(t, &other)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer postgresRollback(tx)
	var raw []byte
	err = tx.QueryRow(ctx, `EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) `+postgresCleanupSQL, s.namespace, cutoff, 16).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	type planNode struct {
		Kind       string     `json:"Node Type"`
		Index      string     `json:"Index Name"`
		Condition  string     `json:"Index Cond"`
		ActualRows float64    `json:"Actual Rows"`
		Plans      []planNode `json:"Plans"`
	}
	var plans []struct{ Plan planNode }
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatal("invalid cleanup plan", err)
	}
	indexed, bounded := false, false
	var visit func(planNode)
	visit = func(node planNode) {
		if node.Index == "session_pg_cleanup_fixture" && strings.Contains(node.Condition, "namespace") && strings.Contains(node.Condition, "deadline_ms") {
			indexed = true
		}
		if node.Kind == "Limit" && node.ActualRows == 16 {
			bounded = true
		}
		for _, child := range node.Plans {
			visit(child)
		}
	}
	visit(plans[0].Plan)
	if !indexed || !bounded {
		t.Fatal("cleanup did not use its bounded namespace/deadline index range", string(raw))
	}
	var remaining int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM session_pg_store_fixture WHERE namespace=$1`, s.namespace).Scan(&remaining); err != nil || remaining != 4016 {
		t.Fatal("EXPLAIN ANALYZE did not delete exactly one bounded batch", remaining, err)
	}
	postgresRollback(tx)
	if postgresRows(t, s, false) != 4032 {
		t.Fatal("plan probe deletion was not rolled back")
	}
	assertPostgresCleanup(t, &other, otherBefore)
	var indexBytes int64
	if err := s.pool.QueryRow(ctx, `SELECT pg_relation_size('session_pg_cleanup_fixture')`).Scan(&indexBytes); err != nil {
		t.Fatal(err)
	}
	t.Logf("cleanup EXPLAIN ANALYZE: target=4000 future+32 due, other=2000 due, batch=16; shared fixture index bytes=%d; plan=%s", indexBytes, raw)
}

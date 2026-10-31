package sessionexperiment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

var costCategories = []string{"success", "expected_reject", "stale", "timeout", "unknown", "error", "missing", "payload_mismatch"}

type costOutcome struct {
	category  string
	latency   int64
	uncertain bool
}

type costResult struct {
	Schema          int                `json:"schema"`
	Backend         string             `json:"backend"`
	Workload        string             `json:"workload"`
	Subjects        int                `json:"subjects"`
	Operations      int                `json:"operations"`
	Concurrency     int                `json:"concurrency"`
	Padding         int                `json:"padding_bytes"`
	FixtureSHA256   string             `json:"fixture_sha256"`
	Valid           bool               `json:"valid"`
	ValidationError string             `json:"validation_error,omitempty"`
	Counts          map[string]int     `json:"counts"`
	Elapsed         int64              `json:"elapsed_ns"`
	Latencies       map[string][]int64 `json:"latency_ns"`
	Quantiles       map[string][]int64 `json:"p50_p95_p99_max_ns"`
	UncertainWrites int                `json:"uncertain_writes"`
	PoolBefore      any                `json:"pool_before"`
	PoolAfter       any                `json:"pool_after"`
	Storage         any                `json:"storage"`
}

func (d *costDataset) request(index, client int) costOutcome {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p := d.fixture.Payloads[index%d.config.subjects]
	token := costExistingToken(index%d.config.subjects, 0)
	if d.config.workload == "admit" || d.config.workload == "reject" {
		token = fmt.Sprintf("new-%06d", index)
	}
	start := time.Now()
	var err error
	if d.config.workload == "find" {
		var found bool
		var b []byte
		b, found, err = d.stores[client].FindCtx(ctx, token)
		if err == nil && !found {
			return costOutcome{category: "missing", latency: time.Since(start).Nanoseconds()}
		}
		if err == nil && !bytes.Equal(b, p.Data) {
			return costOutcome{category: "payload_mismatch", latency: time.Since(start).Nanoseconds()}
		}
	} else {
		err = d.stores[client].CommitCtx(ctx, token, p.Data, time.UnixMilli(d.fixture.Expiry))
	}
	result := costOutcome{category: "success", latency: time.Since(start).Nanoseconds()}
	if err == nil {
		return result
	}
	if errors.Is(err, errLimit) {
		result.category = "error"
		if d.config.workload == "reject" {
			result.category = "expected_reject"
		}
		return result
	}
	if errors.Is(err, errStale) {
		result.category = "stale"
		return result
	}
	result.uncertain = d.config.workload != "find" && !errors.Is(err, errRecovering)
	var netError net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netError) && netError.Timeout()) {
		result.category = "timeout"
	} else if result.uncertain {
		result.category = "unknown"
	} else {
		result.category = "error"
	}
	return result
}

func (d *costDataset) run() (outcomes []costOutcome, elapsed int64) {
	outcomes = make([]costOutcome, d.config.operations)
	var group sync.WaitGroup
	start := time.Now()
	for worker := range costConcurrency {
		group.Go(func() {
			for i := worker; i < d.config.operations; i += costConcurrency {
				outcomes[i] = d.request(i, worker%2)
			}
		})
	}
	group.Wait()
	return outcomes, time.Since(start).Nanoseconds()
}

func summarizeCost(d *costDataset, outcomes []costOutcome, elapsed int64) costResult {
	r := costResult{Schema: 1, Backend: d.config.backend, Workload: d.config.workload,
		Subjects: d.config.subjects, Operations: d.config.operations, Concurrency: costConcurrency,
		Padding: costPadding, FixtureSHA256: d.fixture.SHA256, Elapsed: elapsed,
		Counts: make(map[string]int), Latencies: make(map[string][]int64), Quantiles: make(map[string][]int64)}
	for _, kind := range costCategories {
		r.Counts[kind] = 0
		r.Latencies[kind] = make([]int64, 0)
	}
	for _, result := range outcomes {
		r.Counts[result.category]++
		r.Latencies[result.category] = append(r.Latencies[result.category], result.latency)
		if result.uncertain {
			r.UncertainWrites++
		}
	}
	for kind, values := range r.Latencies {
		if len(values) == 0 {
			r.Quantiles[kind] = []int64{}
			continue
		}
		ordered := append([]int64(nil), values...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
		// Nearest-rank quantiles; retain raw samples instead of averaging p99s.
		for _, percentile := range []int{50, 95, 99, 100} {
			rank := (len(ordered)*percentile + 99) / 100
			r.Quantiles[kind] = append(r.Quantiles[kind], ordered[rank-1])
		}
	}
	target := "success"
	if d.config.workload == "reject" {
		target = "expected_reject"
	}
	r.Valid = r.Counts[target] == d.config.operations && len(outcomes) == d.config.operations
	return r
}

func waitCostFile(ctx context.Context, path string) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func TestSessionCostMeasurement(t *testing.T) {
	if os.Getenv("OJ_SESSION_COST") != "1" {
		t.Skip("opt-in direct Store cost entry; run scripts/test-session-cost.py")
	}
	c, err := parseCostConfig(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	f, err := loadCostFixture("/cost-fixture.json", c.operations)
	if err != nil {
		t.Fatal(err)
	}
	d := openCostDataset(t, c, f)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	// Warm the same client pools on a distinct namespace. These records remain
	// in whole-container/table measurements and are reported explicitly.
	warm := *d
	warm.config.operations = 64
	if c.workload == "admit" {
		warm.config.subjects = 64
	}
	if err := warm.initialize(ctx, digest(c.nonce+"/warm"), warm.config.subjects, c.workload == "admit"); err != nil {
		t.Fatal(err)
	}
	outcomes, elapsed := warm.run()
	if !summarizeCost(&warm, outcomes, elapsed).Valid {
		t.Fatal("warmup did not satisfy the requested Store outcome")
	}
	if err := d.initialize(ctx, digest(c.nonce+"/measured"), c.subjects, c.workload == "admit"); err != nil {
		t.Fatal(err)
	}
	before, err := d.state(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OJ_SESSION_COST_READY", c.nonce)
	if err := waitCostFile(ctx, "/tmp/oj-cost-start"); err != nil {
		t.Fatal(err)
	}
	var beginLSN, endLSN string
	if err := d.pools[0].QueryRow(ctx, `SELECT pg_current_wal_insert_lsn()::text`).Scan(&beginLSN); err != nil {
		t.Fatal(err)
	}
	poolBefore := d.poolStats()
	outcomes, elapsed = d.run()
	poolAfter := d.poolStats()
	if err := d.pools[0].QueryRow(ctx, `SELECT pg_current_wal_insert_lsn()::text`).Scan(&endLSN); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OJ_SESSION_COST_MEASURED", c.nonce)
	if err := waitCostFile(ctx, "/tmp/oj-cost-verify"); err != nil {
		t.Fatal(err)
	}
	r := summarizeCost(d, outcomes, elapsed)
	r.PoolBefore, r.PoolAfter = poolBefore, poolAfter
	after, err := d.state(ctx)
	if err == nil && c.workload == "admit" {
		err = d.validateAdmissions(before, after)
	} else if err == nil && !reflect.DeepEqual(before, after) {
		err = errors.New("read/repeated save/rejection changed session or authority state")
	}
	if err != nil {
		r.Valid, r.ValidationError = false, err.Error()
	}
	r.Storage, err = d.storageStats(ctx, beginLSN, endLSN, warm.config.subjects)
	if err != nil {
		r.Valid, r.ValidationError = false, err.Error()
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("OJ_SESSION_COST_RESULT %s %s\n", c.nonce, b)
	if err := waitCostFile(ctx, "/tmp/oj-cost-ack"); err != nil {
		t.Fatal(err)
	}
	if !r.Valid {
		t.Fatal("invalid Store cost sample")
	}
}

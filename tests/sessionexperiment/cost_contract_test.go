package sessionexperiment

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCostConfigurationRejectsUnsupportedRuns(t *testing.T) {
	base := map[string]string{
		"OJ_SESSION_COST_BACKEND": "redis", "OJ_SESSION_COST_WORKLOAD": "find",
		"OJ_SESSION_COST_NONCE": strings.Repeat("a", 32), "OJ_SESSION_COST_SUBJECTS": "32", "OJ_SESSION_COST_OPS": "2048",
	}
	if _, err := parseCostConfig(func(key string) string { return base[key] }); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"OJ_SESSION_COST_BACKEND": "external", "OJ_SESSION_COST_WORKLOAD": "shell",
		"OJ_SESSION_COST_NONCE": "../../other", "OJ_SESSION_COST_SUBJECTS": "2048", "OJ_SESSION_COST_OPS": "8193",
	} {
		t.Run(key, func(t *testing.T) {
			_, err := parseCostConfig(func(k string) string {
				if k == key {
					return value
				}
				return base[k]
			})
			if err == nil {
				t.Fatal("unsupported cost configuration was accepted")
			}
		})
	}
}

func TestCostFixtureIsFrozenAndValidated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := writeCostFixture(path, strings.Repeat("a", 32), 8); err != nil {
		t.Fatal(err)
	}
	first, err := loadCostFixture(path, 8)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadCostFixture(path, 8)
	if err != nil || !reflect.DeepEqual(first, second) || len(first.SHA256) != 64 {
		t.Fatal("fixture bytes/metadata changed between loads", err)
	}
	if err := writeCostFixture(path, strings.Repeat("b", 32), 8); !errors.Is(err, os.ErrExist) {
		t.Fatal("fixture preparation overwrote an existing artifact", err)
	}
	first.Payloads[0].Data = []byte("malformed Gob")
	b, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(corrupt, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCostFixture(corrupt, 8); err == nil {
		t.Fatal("malformed fixture payload was accepted")
	}
}

func TestCostSummaryRetainsFailureSamples(t *testing.T) {
	d := &costDataset{config: costConfig{backend: "postgres", workload: "find", subjects: 1, operations: 4}, fixture: &costFixture{}}
	values := []costOutcome{{category: "success", latency: 100}, {category: "success", latency: 1}, {category: "success", latency: 3}, {category: "success", latency: 2}}
	r := summarizeCost(d, values, 200)
	if !r.Valid || !reflect.DeepEqual(r.Quantiles["success"], []int64{2, 100, 100, 100}) ||
		!reflect.DeepEqual(r.Latencies["success"], []int64{100, 1, 3, 2}) {
		t.Fatal("nearest-rank quantiles or raw samples were incorrect")
	}
	values[0] = costOutcome{category: "timeout", latency: 1, uncertain: true}
	r = summarizeCost(d, values, 200)
	if r.Valid || r.Counts["success"] != 3 || r.Counts["timeout"] != 1 || r.UncertainWrites != 1 || len(r.Latencies["timeout"]) != 1 {
		t.Fatal("fast failures disappeared or became a valid sample")
	}
}

//go:build sandbox

package sandbox_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

type httpLoopStorage struct {
	id, lifetime string
	baseline     [4]uint64
}

func newHTTPLoopStorage(t *testing.T) *httpLoopStorage {
	t.Helper()
	s := &httpLoopStorage{id: dualDocker(t, "inspect", "--format", "{{.Id}}", container)}
	s.lifetime = s.runningLifetime(t)
	s.baseline = s.sample(t)
	if s.cached(t) != 0 {
		t.Fatal("fixture cache is not empty before Worker startup")
	}
	t.Logf("scratch baseline: %s", formatHTTPStorage(s.baseline))
	return s
}

func (s *httpLoopStorage) runningLifetime(t *testing.T) string {
	t.Helper()
	got := dualDocker(t, "inspect", "--format", "{{.State.Running}} {{.State.StartedAt}} {{.State.Pid}}", s.id)
	fields := strings.Fields(got)
	if len(fields) != 3 || fields[0] != "true" || fields[2] == "0" {
		t.Fatal("missing running fixture lifetime")
	}
	return got
}

func (s *httpLoopStorage) cached(t *testing.T) int {
	t.Helper()
	raw := dualDocker(t, "exec", s.id, "curl", "--disable", "-fsS", "--max-time", "1",
		"http://127.0.0.1:5050/file")
	var entries map[string]string
	if err := json.Unmarshal([]byte(raw), &entries); err != nil || entries == nil {
		t.Fatal("invalid fixture cache inventory")
	}
	return len(entries)
}

func (s *httpLoopStorage) sample(t *testing.T) (out [4]uint64) {
	t.Helper()
	fields := strings.Fields(dualDocker(t, "exec", s.id, "stat", "-f", "-c", "%S %a %d", "/tmp", "/dev/shm"))
	if len(fields) != 6 {
		t.Fatal("invalid fixture filesystem sample")
	}
	for i := range 2 {
		unit, unitErr := strconv.ParseUint(fields[3*i], 10, 64)
		blocks, blocksErr := strconv.ParseUint(fields[3*i+1], 10, 64)
		inodes, inodeErr := strconv.ParseUint(fields[3*i+2], 10, 64)
		if unitErr != nil || blocksErr != nil || inodeErr != nil || unit == 0 || blocks > ^uint64(0)/unit {
			t.Fatal("invalid fixture filesystem counters")
		}
		out[2*i], out[2*i+1] = unit*blocks, inodes
	}
	return out
}

func formatHTTPStorage(v [4]uint64) string {
	return fmt.Sprintf("/tmp available_bytes=%d available_inodes=%d; /dev/shm available_bytes=%d available_inodes=%d",
		v[0], v[1], v[2], v[3])
}

func (s *httpLoopStorage) observeLive(t *testing.T, ctx context.Context, worker *dualProcess) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for ctx.Err() == nil && time.Now().Before(deadline) {
		worker.alive(t)
		if n := s.cached(t); n != 0 {
			if n != 1 {
				t.Fatal("unexpected number of cached executables in the single slot")
			}
			live := s.sample(t)
			if s.runningLifetime(t) != s.lifetime {
				t.Fatal("fixture restarted during cached executable observation")
			}
			if live[2] < s.baseline[2] && live[3] < s.baseline[3] {
				t.Logf("scratch with live cached executable: %s", formatHTTPStorage(live))
				return
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("real Worker cached executable and shared memory usage were not observed")
}

func (s *httpLoopStorage) observeClean(t *testing.T) {
	t.Helper()
	if s.runningLifetime(t) != s.lifetime || s.cached(t) != 0 {
		t.Fatal("terminal attempts left cache entries or replaced the slot")
	}
	after := s.sample(t)
	t.Logf("scratch after terminal attempts, before drain: %s", formatHTTPStorage(after))
	if after != s.baseline {
		t.Fatal("scratch filesystem counters did not return to their baseline")
	}
}

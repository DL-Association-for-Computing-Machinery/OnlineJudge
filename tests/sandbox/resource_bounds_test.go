//go:build sandbox

package sandbox_test

import (
	_ "embed"
	"encoding/json"
	"os"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

//go:embed testdata/resource_probe.cpp
var resourceProbe string

type storageSnapshot struct {
	Bytes      uint64 `json:"bytes"`
	Available  uint64 `json:"available"`
	Inodes     uint64 `json:"inodes"`
	FreeInodes uint64 `json:"free_inodes"`
	Tmpfs      bool   `json:"tmpfs"`
}

func TestSandboxResourceBounds(t *testing.T) {
	if os.Getenv("OJ_SANDBOX_RESOURCES") != "1" {
		t.Skip("run python3 scripts/test-worker.py resource-bounds")
	}
	verifyEnvironment(t)
	p := experiment.PinnedEnvironment()
	if p.Run.Processes != 16 || p.Run.Memory != 32<<20 || p.Compile.Memory != 512<<20 {
		t.Fatal("resource evidence requires the original pinned budgets")
	}
	t.Run("bounded probes", func(t *testing.T) {
		id, compiled := compile(t, resourceProbe)
		if compiled.Status != "Accepted" || compiled.Exit != 0 || id == "" {
			t.Fatalf("resource probe compilation: %+v", compiled)
		}
		probe := func(t *testing.T, mode string, limits experiment.Limits) outcome {
			t.Helper()
			command := base(p.RunArgs, mode)
			command["cpuLimit"], command["clockLimit"] = limits.CPU, limits.Wall
			command["memoryLimit"], command["procLimit"] = limits.Memory, limits.Processes
			command["copyIn"] = map[string]any{"main": map[string]any{"fileId": id}}
			return execute(t, command)
		}
		decode := func(t *testing.T, r outcome, target any) {
			t.Helper()
			if r.Status != "Accepted" || r.Exit != 0 || r.Error != "" || len(r.FileErrors) != 0 || len(r.IDs) != 0 {
				t.Fatalf("probe failed: %+v", r)
			}
			if err := json.Unmarshal([]byte(r.Files["stdout"]), target); err != nil {
				t.Fatalf("probe evidence: %v: %+v", err, r)
			}
		}
		var baseline storageSnapshot
		decode(t, probe(t, "clean", p.Run), &baseline)
		if !baseline.Tmpfs || baseline.Bytes != 64<<20 || baseline.Inodes != 4096 || baseline.Available == 0 || baseline.FreeInodes < 4000 {
			t.Fatalf("unexpected isolated filesystem: %+v", baseline)
		}
		fresh := func(t *testing.T) {
			t.Helper()
			var got storageSnapshot
			decode(t, probe(t, "clean", p.Run), &got)
			if got != baseline {
				t.Fatalf("per-execution storage was not restored: got %+v want %+v", got, baseline)
			}
		}
		t.Run("process count and reuse", func(t *testing.T) {
			var evidence struct {
				Children  int    `json:"children"`
				Error     int    `json:"error"`
				Eagain    bool   `json:"eagain"`
				Reaped    int    `json:"reaped"`
				Reused    bool   `json:"reused"`
				NprocSoft uint64 `json:"nproc_soft"`
			}
			r := probe(t, "processes", p.Run)
			decode(t, r, &evidence)
			if evidence.Children != p.Run.Processes-1 || !evidence.Eagain || evidence.Reaped != evidence.Children || !evidence.Reused || evidence.NprocSoft <= 32 || r.ProcPeak != p.Run.Processes {
				t.Fatalf("no exact process-budget/reuse evidence: %+v peak=%d", evidence, r.ProcPeak)
			}
			t.Logf("parent+%d blocked children, cgroup peak=%d; next fork errno=%d/EAGAIN, all reaped and capacity reused; cpu=%d memory=%d", evidence.Children, r.ProcPeak, evidence.Error, r.Time, r.Memory)
			fresh(t)
		})
		t.Run("inode exhaustion and teardown", func(t *testing.T) {
			var evidence struct {
				Before, Full, Reused storageSnapshot
				Count                uint64
				Enospc               bool
				Error                int
			}
			r := probe(t, "inodes", p.Run)
			decode(t, r, &evidence)
			if evidence.Before != baseline || !evidence.Enospc || evidence.Count+1 != baseline.FreeInodes || evidence.Full.FreeInodes != 0 || evidence.Full.Available == 0 || evidence.Reused != evidence.Full {
				t.Fatalf("no independent inode exhaustion/reuse evidence: %+v", evidence)
			}
			t.Logf("%d empty files plus directory exhausted inodes, errno=%d/ENOSPC; bytes still available=%d; cpu=%d memory=%d", evidence.Count, evidence.Error, evidence.Full.Available, r.Time, r.Memory)
			fresh(t)
		})
		for _, mode := range []string{"bytes-full", "bytes-reuse"} {
			t.Run(mode+" with existing compile budget", func(t *testing.T) {
				var evidence struct {
					Before, Full, Released storageSnapshot
					Written                uint64
					FileLimit              uint64 `json:"file_limit"`
					Count                  uint64
					Enospc                 bool
					Error                  int
				}
				// Direct sandbox probe using existing compile resource limits.
				// The server's per-file cap and collectors stay unchanged.
				r := probe(t, mode, p.Compile)
				decode(t, r, &evidence)
				if evidence.Before != baseline || !evidence.Enospc || evidence.Written != baseline.Available || evidence.Full.Available != 0 || evidence.Full.FreeInodes == 0 || evidence.FileLimit != 1<<20 || evidence.Count == 0 || evidence.Count > 80 || evidence.Full.FreeInodes+evidence.Count+1 != baseline.FreeInodes {
					t.Fatalf("no independent byte exhaustion evidence: %+v", evidence)
				}
				if mode == "bytes-reuse" && (evidence.Released.Available != baseline.Available || evidence.Released.FreeInodes+1 != baseline.FreeInodes) {
					t.Fatalf("closed/unlinked storage was not released: %+v", evidence)
				}
				t.Logf("compile envelope: %d real bytes written, errno=%d/ENOSPC, free inodes=%d; cpu=%d memory=%d", evidence.Written, evidence.Error, evidence.Full.FreeInodes, r.Time, r.Memory)
				fresh(t)
			})
		}
		t.Run("ordinary single file size bound", func(t *testing.T) {
			r := probe(t, "file-size", p.Run)
			var evidence struct {
				FileLimit uint64 `json:"file_limit"`
			}
			if r.Status != "Output Limit Exceeded" || r.Exit != 25 || r.Error != "" || len(r.FileErrors) != 0 || len(r.IDs) != 0 || json.Unmarshal([]byte(r.Files["stdout"]), &evidence) != nil || evidence.FileLimit != 1<<20 {
				t.Fatalf("no unchanged RLIMIT_FSIZE/SIGXFSZ evidence: %+v", r)
			}
			t.Logf("ordinary file cap=%d stopped finite write as SIGXFSZ/OLE with zero fileError; cpu=%d memory=%d", evidence.FileLimit, r.Time, r.Memory)
			fresh(t)
		})
		t.Run("ordinary run memory takes precedence", func(t *testing.T) {
			r := probe(t, "bytes-full", p.Run)
			if r.Status != "Memory Limit Exceeded" {
				t.Fatalf("ordinary write did not respect unchanged Run memory budget: %+v", r)
			}
			t.Logf("ordinary envelope stopped as MLE before ENOSPC acknowledgement; cpu=%d memory=%d", r.Time, r.Memory)
			fresh(t)
		})
	})
	t.Run("fixed compiler file size", testCompileFileSize)
	var cache map[string]string
	if err := json.Unmarshal(api(t, "GET", "/file", nil), &cache); err != nil || len(cache) != 0 {
		t.Fatalf("probe artifact cleanup: entries=%d error=%v", len(cache), err)
	}
}

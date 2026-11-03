//go:build sandbox

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestUnionFindReferenceSandbox(t *testing.T) {
	if os.Getenv("OJ_UNIONFIND_VALIDATION") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv("OJ_WORKER_M0_SLOT") != "" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("explicit independent Desktop reference experiment required")
	}
	t.Run("complete-eighteen-point-cli", func(t *testing.T) {
		bundle, output := os.Getenv("OJ_REFERENCE_BUNDLE"), os.Getenv("OJ_UNIONFIND_OUTPUT")
		if !canonicalAbsolute(bundle) || !canonicalAbsolute(output) {
			t.Fatal("explicit complete bundle and new output required")
		}
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		binary, err = filepath.EvalSymlinks(binary)
		if err != nil {
			t.Fatal(err)
		}
		root := filepath.Clean(filepath.Join(filepath.Dir(binary), "../.."))
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(root, ".local/validation-host/worker"), "validate-reference", "--source-id", "library-checker:unionfind", "--bundle", bundle, "--source-lock", filepath.Join(root, "config/problem-sources/library-checker.json"), "--output", output, "--start-stopped-slot")
		raw, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("real CLI: %v: %s", err, raw)
		}
		raw, err = os.ReadFile(filepath.Join(output, "report.json"))
		if err != nil {
			t.Fatal(err)
		}
		var report problemvalidation.Report
		if err := json.Unmarshal(raw, &report); err != nil {
			t.Fatal(err)
		}
		if report.Status != "reference-validation-passed" || report.Verdict != "AC" || !report.StopConfirmed || report.Publication != "blocked" || report.Slot != (workerhost.ReferenceSlot{}).ContainerName() || len(report.Points) != 18 || len(report.Plan.Cases) != 18 || report.Plan.SourceID != "library-checker:unionfind" || report.Plan.SourceLockSHA256 != problemvalidation.SourceLockSHA256 || report.ProfileSHA256 != problemvalidation.UnionFindProfile().Digest() || report.PlanSHA256 != report.Plan.Digest() {
			t.Fatal("incomplete identity/profile/pass evidence")
		}
		for _, p := range report.Points {
			if p.Verdict != "AC" || p.Usage == nil {
				t.Fatal("unexecuted point")
			}
		}
		if _, err := os.Stat(filepath.Join(output, "INCOMPLETE")); !os.IsNotExist(err) {
			t.Fatal("incomplete marker")
		}
		lock, err := lockSlotMode((workerhost.ReferenceSlot{}).LockName(), false)
		if err != nil {
			t.Fatal("reference ownership not clean", err)
		}
		lock.Close()
		t.Logf("full unionfind draft=%s import=%s plan=%s profile=%s; all 18 AC, stop confirmed, publication blocked", report.Plan.Version, report.Plan.ImportRevision, report.PlanSHA256, report.ProfileSHA256)
	})
	t.Run("raw-output-and-diagnostic-bounds", func(t *testing.T) {
		h := unionSandboxHost(t)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		p := experiment.PinnedReferenceEnvironment()
		g := execution.NewReferenceGoJudge(referenceProbeTransport{h, t}, h.Verify, h)
		if err := g.Verify(ctx, p); err != nil {
			t.Fatal(err)
		}
		o, err := g.Compile(ctx, p, `#include <cstdio>
int main(){int n;if(scanf("%d",&n)!=1)return 3;for(int i=0;i<n;i++)putchar('x');}`)
		if err != nil || o.Artifact == "" {
			t.Fatal("probe compile", err)
		}
		for _, size := range []int{65536, p.OutputBytes, p.OutputBytes + 1} {
			out, err := g.Run(ctx, p, o.Artifact, []byte(strconv.Itoa(size)))
			if err != nil {
				t.Fatal("output probe", size, err)
			}
			v, err := execution.Classify(out, false)
			if err != nil {
				t.Fatal(err)
			}
			if size <= p.OutputBytes {
				if v != "AC" || !bytes.Equal(out.Stdout, bytes.Repeat([]byte{'x'}, size)) {
					t.Fatal("raw output changed")
				}
			} else if v != "OLE" {
				t.Fatal("overflow accepted")
			}
			t.Logf("actual stdout request=%d returned=%d verdict=%s CPU=%d memory=%d", size, len(out.Stdout), v, out.CPU, out.Memory)
		}
		if err := g.Remove(ctx, o.Artifact); err != nil {
			t.Fatal(err)
		}
		referenceStorageSample(t, h, "reference-after-cleanup")
	})
}

// This trusted synthetic probe records fixed comparisons, never raw executor
// errors or stream bytes, to distinguish protocol evidence from a test guess.
type referenceProbeTransport struct {
	host http.RoundTripper
	t    *testing.T
}

func (p referenceProbeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := p.host.RoundTrip(req)
	if err != nil || req.URL.Path != "/run" {
		return resp, err
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024+1))
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Exit   *int   `json:"exitStatus"`
		CPU    *int64 `json:"time"`
		Memory *int64 `json:"memory"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.Error != "" {
			p.t.Logf("actual collector error_label_matches=%v OLE_status=%v normal_exit=%v usage_present=%v", r.Error == "Output Limit Exceeded", r.Status == "Output Limit Exceeded", r.Exit != nil && *r.Exit == 0, r.CPU != nil && r.Memory != nil)
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	return resp, nil
}

func unionSandboxHost(t *testing.T) *workerhost.Host {
	t.Helper()
	lock, err := lockSlotMode((workerhost.ReferenceSlot{}).LockName(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.dirty(); err != nil {
		lock.Close()
		t.Fatal(err)
	}
	h, openErr := workerhost.StartReference(context.Background())
	t.Cleanup(func() {
		defer lock.Close()
		if h == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := h.Stop(ctx); err != nil {
			t.Error("reference stop unconfirmed; keep quarantine", err)
			return
		}
		if err := lock.clean(); err != nil {
			t.Error(err)
		}
	})
	if openErr != nil || h == nil {
		t.Fatal("reference start", openErr)
	}
	return h
}

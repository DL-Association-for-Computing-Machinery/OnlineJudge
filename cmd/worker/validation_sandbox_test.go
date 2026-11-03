//go:build sandbox

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

// This suite is intentionally separate from ordinary CI and from M0's database
// driver. All starts and stops happen under the actual canonical slot lock.
func TestPrivateDraftReferenceSandbox(t *testing.T) {
	if os.Getenv("OJ_REFERENCE_VALIDATION") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("requires explicit dedicated Desktop reference-validation experiment")
	}
	slot, err := workerhost.ParseM0Slot(os.Getenv("OJ_WORKER_M0_SLOT"))
	if err != nil {
		t.Fatal(err)
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
	bundle := os.Getenv("OJ_REFERENCE_BUNDLE")
	if !canonicalAbsolute(bundle) {
		t.Fatal("explicit private complete OJ_REFERENCE_BUNDLE required")
	}
	t.Run("real-twelve-point-cli", func(t *testing.T) {
		output := os.Getenv("OJ_REFERENCE_OUTPUT")
		if !canonicalAbsolute(output) {
			t.Fatal("explicit new private OJ_REFERENCE_OUTPUT required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(root, ".local/validation-host/worker"), "validate-reference", "--bundle", bundle, "--source-lock", filepath.Join(root, "config/problem-sources/library-checker.json"), "--output", output, "--start-stopped-slot")
		cmd.Env = os.Environ()
		raw, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("real CLI: %v: %s", err, raw)
		}
		reportBytes, err := os.ReadFile(filepath.Join(output, "report.json"))
		if err != nil {
			t.Fatal(err)
		}
		var report problemvalidation.Report
		if err := json.Unmarshal(reportBytes, &report); err != nil {
			t.Fatal(err)
		}
		if report.Status != "reference-validation-passed" || report.Verdict != "AC" || !report.StopConfirmed || report.Publication != "blocked" || len(report.Points) != 12 || report.Plan.SourceID != "library-checker:aplusb" || report.Plan.SourceLockSHA256 != problemvalidation.SourceLockSHA256 || report.ProfileSHA256 != problemvalidation.PinnedProfile().Digest() {
			t.Fatalf("incomplete real report: %+v", report)
		}
		if report.Plan.Definition.SHA256 == "" || report.Plan.Hashes.SHA256 == "" || report.PlanSHA256 != report.Plan.Digest() || report.ProfileSHA256 != report.Profile.Digest() {
			t.Fatal("source catalog or complete plan/profile binding missing")
		}
		for _, p := range report.Points {
			if p.Verdict != "AC" || p.Usage == nil {
				t.Fatal("unexecuted real point")
			}
		}
		if _, err := os.Stat(filepath.Join(output, "INCOMPLETE")); !os.IsNotExist(err) {
			t.Fatal("incomplete report")
		}
		t.Logf("real immutable draft=%s import=%s plan=%s profile=%s all %d points AC, stop confirmed, publication blocked", report.Plan.Version, report.Plan.ImportRevision, report.PlanSHA256, report.ProfileSHA256, len(report.Points))
	})
	for _, c := range []struct{ name, source, want string }{
		{"AC", `#include <cstdio>
int main(){long long a,b;scanf("%lld%lld",&a,&b);printf("%lld",a+b);}`, "AC"},
		{"WA", `#include <cstdio>
int main(){puts("wrong");}`, "WA"},
		{"CE", `this is not C++`, "CE"},
		{"RE", `int main(){return 1;}`, "RE"},
		{"TLE", `int main(){for(;;){asm volatile("");}}`, "TLE"},
		{"MLE", `#include <cstdlib>
int main(){volatile char* p=(char*)malloc(64*1024*1024);if(!p)return 4;for(int i=0;i<64*1024*1024;i+=4096)p[i]=1;}`, "MLE"},
		{"OLE", `#include <cstdio>
int main(){for(;;)puts("bounded output");}`, "OLE"},
	} {
		t.Run("synthetic-"+c.name, func(t *testing.T) {
			b := problemfixture.New(t)
			b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", []byte(c.source))
			first := b.Document["test_cases"].([]any)[0].(map[string]any)
			b.Document["test_cases"] = append(b.Document["test_cases"].([]any), map[string]any{"id": "case-0002", "input": first["input"], "answer": first["answer"], "sample": false})
			provenance := b.Document["provenance"].(map[string]any)
			provenance["source_case_mapping"] = append(provenance["source_case_mapping"].([]any), map[string]any{"case_id": "case-0002", "upstream_case": "random_00"})
			b.BindReferenceCases(t)
			b.Write(t)
			p, err := problem.LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = p.Close() }()
			lock, err := lockM0(slot)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			r, err := runReferenceLocked(context.Background(), p, lock, func(ctx context.Context) (validationHost, error) {
				h, e := workerhost.StartM0Slot(ctx, slot)
				if h == nil {
					return nil, e
				}
				return h, e
			})
			if err != nil || r.Verdict != c.want || !r.StopConfirmed || (r.Status == "reference-validation-passed") != (c.want == "AC") {
				t.Fatalf("%s: verdict=%s status=%s stop=%v error=%v", c.name, r.Verdict, r.Status, r.StopConfirmed, err)
			}
			if len(r.Points) != 2 {
				t.Fatal("incomplete synthetic case list")
			}
			if c.want != "AC" && r.Points[1].Verdict != "NOT_RUN" {
				t.Fatal("ran later point after reference failure")
			}
			points, e := json.Marshal(r.Points)
			if e != nil {
				t.Fatal(e)
			}
			t.Logf("actual %s: verdict=%s points=%s stop-confirmed", c.name, r.Verdict, points)
		})
	}
	t.Run("cancelled-dispatch", func(t *testing.T) {
		b := problemfixture.New(t)
		b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", []byte(`int main(){for(;;){asm volatile("");}}`))
		b.BindReferenceCases(t)
		b.Write(t)
		p, err := problem.LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = p.Close() }()
		lock, err := lockM0(slot)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		if err := lock.dirty(); err != nil {
			t.Fatal(err)
		}
		h, err := workerhost.StartM0Slot(context.Background(), slot)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		transport := &cancelReferenceTransport{host: h, cancel: cancel}
		r, runErr := problemvalidation.New(execution.NewCachedGoJudge(transport, h.Verify, h)).Validate(ctx, p, problemvalidation.PinnedProfile())
		stopCtx, done := context.WithTimeout(context.Background(), 15*time.Second)
		stopErr := h.Stop(stopCtx)
		done()
		var cleanErr error
		if stopErr == nil {
			cleanErr = lock.clean()
		}
		err = r.Finalize(runErr, stopErr, cleanErr)
		if !errors.Is(err, context.Canceled) && runErr == nil {
			t.Fatal("cancellation did not fail validation")
		}
		if !transport.dispatched || r.Status == "reference-validation-passed" || !r.StopConfirmed || stopErr != nil || cleanErr != nil {
			t.Fatalf("cancel/stop evidence: %+v err=%v", r, err)
		}
		if err := h.Verify(context.Background(), problemvalidation.PinnedProfile().Runtime); err == nil {
			t.Fatal("stopped host reused")
		}
	})
}

type cancelReferenceTransport struct {
	host       *workerhost.Host
	cancel     context.CancelFunc
	runs       int
	dispatched bool
}

func (t *cancelReferenceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == "/run" {
		t.runs++
		if t.runs == 2 {
			t.dispatched = true
			timer := time.AfterFunc(100*time.Millisecond, t.cancel)
			defer timer.Stop()
		}
	}
	return t.host.RoundTrip(r)
}

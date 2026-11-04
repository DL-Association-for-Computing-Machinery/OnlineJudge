//go:build sandbox

package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestPointAddReferenceSandbox(t *testing.T) {
	if os.Getenv("OJ_POINT_ADD_REFERENCE") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv("OJ_WORKER_M0_SLOT") != "" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("explicit independent Desktop point-add reference experiment required")
	}
	if !t.Run("complete-twenty-one-point-cli", func(t *testing.T) {
		referenceSourceCLI(t, "library-checker:point_add_range_sum", "OJ_POINT_ADD_OUTPUT", 21)
	}) {
		return
	}
	t.Run("sixteen-MiB-cached-stdin", func(t *testing.T) {
		h := referenceTierSandboxHost(t, experiment.ReferenceMediumBulk)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		c, err := experiment.ResolveReferenceTier(experiment.ReferenceMediumBulk)
		if err != nil {
			t.Fatal(err)
		}
		files := &bulkMeasuredFiles{Host: h, t: t}
		g, err := execution.NewReferenceGoJudgeForTier(h, h.Verify, files, experiment.ReferenceMediumBulk)
		if err != nil {
			t.Fatal(err)
		}
		verifyCtx, verifyDone := context.WithTimeout(ctx, 10*time.Second)
		verifyErr := g.Verify(verifyCtx, c.Runtime)
		verifyDone()
		if verifyErr != nil {
			t.Fatal(verifyErr)
		}
		compileCtx, compileDone := context.WithTimeout(ctx, time.Duration(c.Runtime.Compile.Wall)+5*time.Second)
		o, err := g.Compile(compileCtx, c.Runtime, `#include <cstdio>
int main(){unsigned char b[65536];unsigned long long n=0,sum=0;size_t k;
while((k=fread(b,1,sizeof(b),stdin))){n+=k;for(size_t i=0;i<k;i++)sum+=b[i];}
if(ferror(stdin))return 3;printf("%llu %llu\n",n,sum);}`)
		compileDone()
		if err != nil || o.Artifact == "" {
			t.Fatal("large stdin compile", err)
		}
		for _, size := range []int{(8 << 20) + 1, 16 << 20} {
			input := make([]byte, size)
			var sum uint64
			for i := range input {
				input[i] = byte(i)
				sum += uint64(input[i])
			}
			// Match the Runner's shared upload/readback/run/stdout deadline.
			runCtx, runDone := context.WithTimeout(ctx, time.Duration(c.Runtime.Run.Wall)+5*time.Second)
			out, err := g.Run(runCtx, c.Runtime, o.Artifact, input)
			runDone()
			v, classifyErr := execution.Classify(out, false)
			if err != nil || classifyErr != nil || v != "AC" || string(out.Stdout) != fmt.Sprintf("%d %d\n", size, sum) {
				t.Fatal("large cached stdin under 8MiB output RLIMIT", size, err, classifyErr)
			}
			t.Logf("cached stdin=%d output=%q verdict=%s CPU=%d memory=%d", size, out.Stdout, v, out.CPU, out.Memory)
		}
		if !files.sampled {
			t.Fatal("large input resource sample missing")
		}
		cleanupCtx, cleanupDone := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupDone()
		if err := g.Remove(cleanupCtx, o.Artifact); err != nil {
			t.Fatal(err)
		}
		referenceStorageSample(t, h, "bulk-stdin-after-cleanup")
	})
}

type bulkMeasuredFiles struct {
	*workerhost.Host
	t       *testing.T
	sampled bool
}

func (f *bulkMeasuredFiles) ReadFile(ctx context.Context, id string, maximum int64) ([]byte, error) {
	raw, err := f.Host.ReadFile(ctx, id, maximum)
	if err == nil && !f.sampled && len(raw) == 16<<20 {
		f.sampled = true
		referenceStorageSample(f.t, f.Host, "bulk-input-cache-live")
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		f.t.Logf("large input host Go heap stage alloc=%d inuse=%d sys=%d bytes; not process RSS peak", m.HeapAlloc, m.HeapInuse, m.Sys)
	}
	return raw, err
}

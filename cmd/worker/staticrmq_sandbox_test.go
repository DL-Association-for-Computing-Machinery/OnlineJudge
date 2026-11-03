//go:build sandbox

package main

import (
	"bytes"
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

func TestStaticRMQReferenceSandbox(t *testing.T) {
	if os.Getenv("OJ_STATICRMQ_REFERENCE") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv("OJ_WORKER_M0_SLOT") != "" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("explicit independent Desktop staticrmq experiment required")
	}
	if !t.Run("cached-sixteen-MiB-stdin", func(t *testing.T) {
		h := referenceTierSandboxHost(t, experiment.ReferenceStaticRMQ)
		c, err := experiment.ResolveReferenceTier(experiment.ReferenceStaticRMQ)
		if err != nil {
			t.Fatal(err)
		}
		files := &staticRMQMeasuredFiles{host: h, t: t}
		g, err := execution.NewReferenceGoJudgeForTier(h, h.Verify, files, experiment.ReferenceStaticRMQ)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := g.Verify(ctx, c.Runtime); err != nil {
			t.Fatal(err)
		}
		referenceStorageSample(t, h, "staticrmq-before-upload")
		compileCtx, compileDone := context.WithTimeout(ctx, time.Duration(c.Runtime.Compile.Wall)+5*time.Second)
		o, err := g.Compile(compileCtx, c.Runtime, `#include <cstdio>
#include <cstring>
int main(){unsigned char b[8192],want[8192];for(int i=0;i<8192;i++)want[i]=(unsigned char)i;
unsigned long long total=0;size_t n;while((n=fread(b,1,sizeof b,stdin))!=0){if(memcmp(b,want,n))return 7;total+=n;}
printf("%llu\n",total);}`)
		compileDone()
		verdict, classifyErr := execution.Classify(o, true)
		if err != nil || classifyErr != nil || verdict != "AC" || o.Artifact == "" {
			t.Fatal("trusted input probe compilation", execution.FailureDetails(err))
		}
		pattern := make([]byte, 256)
		for i := range pattern {
			pattern[i] = byte(i)
		}
		input := bytes.Repeat(pattern, (16<<20)/len(pattern))
		// Match the ordinary Runner wrapper; larger transport is not a reason to
		// extend execution, file-operation or Stop deadlines.
		runCtx, runDone := context.WithTimeout(ctx, time.Duration(c.Runtime.Run.Wall)+5*time.Second)
		out, err := g.Run(runCtx, c.Runtime, o.Artifact, input)
		runDone()
		verdict, classifyErr = execution.Classify(out, false)
		if err != nil || classifyErr != nil || verdict != "AC" || string(out.Stdout) != fmt.Sprintf("%d\n", len(input)) || !files.sampled {
			t.Fatal("sixteen-MiB cached stdin byte/length verification", execution.FailureDetails(err))
		}
		cleanupCtx, cleanupDone := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupDone()
		if err := g.Remove(cleanupCtx, o.Artifact); err != nil {
			t.Fatal(err)
		}
		referenceStorageSample(t, h, "staticrmq-after-cleanup")
		t.Logf("trusted stdin=%d all-byte pattern and total length verified; CPU=%d memory=%d", len(input), out.CPU, out.Memory)
	}) {
		return
	}
	t.Run("complete-twenty-seven-point-cli", func(t *testing.T) {
		referenceSourceCLI(t, "library-checker:staticrmq", "OJ_STATICRMQ_OUTPUT", 27)
	})
}

type staticRMQMeasuredFiles struct {
	host    *workerhost.Host
	t       *testing.T
	sampled bool
}

func (f *staticRMQMeasuredFiles) UploadFile(ctx context.Context, raw []byte) (string, error) {
	id, err := f.host.UploadFile(ctx, raw)
	if err == nil && !f.sampled && len(raw) == 16<<20 {
		f.sampled = true
		referenceStorageSample(f.t, f.host, "staticrmq-stdin-cache-live")
	}
	return id, err
}

func (f *staticRMQMeasuredFiles) ReadFile(ctx context.Context, id string, maximum int64) ([]byte, error) {
	return f.host.ReadFile(ctx, id, maximum)
}

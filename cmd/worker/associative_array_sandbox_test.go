//go:build sandbox

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestAssociativeArrayReferenceSandbox(t *testing.T) {
	if os.Getenv("OJ_ASSOCIATIVE_ARRAY_REFERENCE") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv("OJ_WORKER_M0_SLOT") != "" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("explicit independent Desktop associative-array experiment required")
	}
	if !t.Run("same-run-input48MiB-output8MiB", testAssociativeArrayCombinedBoundary) {
		return
	}
	t.Run("complete-twenty-point-cli", func(t *testing.T) {
		referenceSourceCLI(t, "library-checker:associative_array", "OJ_ASSOCIATIVE_ARRAY_OUTPUT", 20)
	})
}

func testAssociativeArrayCombinedBoundary(t *testing.T) {
	h := referenceTierSandboxHost(t, experiment.ReferenceAssociativeArray)
	c, err := experiment.ResolveReferenceTier(experiment.ReferenceAssociativeArray)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	wire := &staticRangeSumProbeTransport{host: h}
	files := &associativeMeasuredFiles{host: h, t: t}
	g, err := execution.NewReferenceGoJudgeForTier(wire, h.Verify, files, experiment.ReferenceAssociativeArray)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Verify(ctx, c.Runtime); err != nil {
		t.Fatal(err)
	}
	compileCtx, compileDone := context.WithTimeout(ctx, time.Duration(c.Runtime.Compile.Wall)+5*time.Second)
	o, err := g.Compile(compileCtx, c.Runtime, `#include <cstdio>
#include <unistd.h>
int main(){int n;if(scanf("%d",&n)!=1||getchar()!='\n')return 3;
unsigned char b[65536];int total=0;
while(total<n){int k=n-total;if(k>65536)k=65536;if(fread(b,1,k,stdin)!=(size_t)k)return 4;
for(int i=0;i<k;i++)if(b[i]!=(unsigned char)(total+i))return 5;total+=k;}
if(getchar()!=EOF)return 6;for(int i=0;i<65536;i++)b[i]=(unsigned char)i;
char diagnostic[4096];for(int i=0;i<4096;i++)diagnostic[i]='e';
if(write(2,diagnostic,4096)!=4096)return 7;
for(int n=0;n<128;n++)if(write(1,b,65536)!=65536)return 8;}
`)
	compileDone()
	verdict, classifyErr := execution.Classify(o, true)
	if err != nil || classifyErr != nil || verdict != "AC" || o.Artifact == "" {
		t.Fatal("trusted combined boundary compilation", execution.FailureDetails(err))
	}
	pattern := make([]byte, 256)
	for i := range pattern {
		pattern[i] = byte(i)
	}
	bodyBytes := (48 << 20) - len(strconv.Itoa(48<<20)+"\n")
	input := []byte(strconv.Itoa(bodyBytes) + "\n")
	if len(input)+bodyBytes != 48<<20 {
		t.Fatal("exact input framing changed")
	}
	input = append(input, bytes.Repeat(pattern, (bodyBytes+255)/256)[:bodyBytes]...)
	// Match the real runner's single shared upload/readback/run/stdout deadline.
	runCtx, runDone := context.WithTimeout(ctx, time.Duration(c.Runtime.Run.Wall)+5*time.Second)
	out, err := g.Run(runCtx, c.Runtime, o.Artifact, input)
	runDone()
	verdict, classifyErr = execution.Classify(out, false)
	actual := wire.last
	want := bytes.Repeat(pattern, (8<<20)/len(pattern))
	if err != nil || classifyErr != nil || verdict != "AC" || !bytes.Equal(out.Stdout, want) || out.OutputTruncated || !files.inputSample || !files.outputSample {
		t.Fatal("same-run 48MiB input and 8MiB output", execution.FailureDetails(err))
	}
	if !actual.accepted || actual.exit != 0 || !actual.completeUsage || !actual.onlyStdoutID || !actual.onlyStderrInline || !actual.stderrAllE || actual.stderrBytes != 4096 || actual.errorPresent || actual.fileErrors != 0 {
		t.Fatal("incomplete trusted combined wire evidence")
	}
	t.Logf("same-run input=%d stdout=%d SHA256=%x stderr=%d CPU=%d memory=%d", len(input), len(out.Stdout), sha256.Sum256(out.Stdout), actual.stderrBytes, out.CPU, out.Memory)
	cleanupCtx, cleanupDone := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupDone()
	if err := g.Remove(cleanupCtx, o.Artifact); err != nil {
		t.Fatal("trusted executable cleanup", execution.FailureDetails(err))
	}
	referenceStorageSample(t, h, "associative-after-cleanup")
}

type associativeMeasuredFiles struct {
	host                      *workerhost.Host
	t                         *testing.T
	inputSample, outputSample bool
}

func (f *associativeMeasuredFiles) UploadFile(ctx context.Context, raw []byte) (string, error) {
	id, err := f.host.UploadFile(ctx, raw)
	if err == nil && len(raw) == 48<<20 {
		f.inputSample = true
		referenceStorageSample(f.t, f.host, "associative-input-live")
	}
	return id, err
}

func (f *associativeMeasuredFiles) ReadFile(ctx context.Context, id string, maximum int64) ([]byte, error) {
	raw, err := f.host.ReadFile(ctx, id, maximum)
	if err == nil && maximum == (8<<20)+1 && len(raw) == 8<<20 {
		f.outputSample = true
		referenceStorageSample(f.t, f.host, "associative-output-live")
	}
	return raw, err
}

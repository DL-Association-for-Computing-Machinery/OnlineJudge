//go:build sandbox

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestStaticRangeSumReferenceSandbox(t *testing.T) {
	if os.Getenv("OJ_STATIC_RANGE_SUM_REFERENCE") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv("OJ_WORKER_M0_SLOT") != "" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("explicit independent Desktop static range sum experiment required")
	}
	if !t.Run("raw-eight-MiB-and-narrow-output-evidence", testStaticRangeSumOutputBoundary) {
		return
	}
	t.Run("complete-eleven-point-cli", func(t *testing.T) {
		referenceSourceCLI(t, "library-checker:static_range_sum", "OJ_STATIC_RANGE_SUM_OUTPUT", 11)
	})
}

func testStaticRangeSumOutputBoundary(t *testing.T) {
	h := referenceTierSandboxHost(t, experiment.ReferenceStaticRangeSum)
	c, err := experiment.ResolveReferenceTier(experiment.ReferenceStaticRangeSum)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	wire, files := &staticRangeSumProbeTransport{host: h}, &staticRangeSumMeasuredFiles{host: h, t: t}
	g, err := execution.NewReferenceGoJudgeForTier(wire, h.Verify, files, experiment.ReferenceStaticRangeSum)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Verify(ctx, c.Runtime); err != nil {
		t.Fatal(err)
	}
	compileCtx, compileDone := context.WithTimeout(ctx, time.Duration(c.Runtime.Compile.Wall)+5*time.Second)
	o, err := g.Compile(compileCtx, c.Runtime, `#include <cstdio>
#include <unistd.h>
int main(){int n,e,input;if(scanf("%d %d %d",&n,&e,&input)!=3||getchar()!='\n')return 3;
unsigned char b[65536];int total=0;
while(total<input){int k=input-total;if(k>65536)k=65536;if(fread(b,1,k,stdin)!=(size_t)k)return 4;
for(int i=0;i<k;i++)if(b[i]!=(unsigned char)(total+i))return 5;total+=k;}
if(getchar()!=EOF)return 6;for(int i=0;i<65536;i++)b[i]=(unsigned char)i;
char diagnostics[4097];for(int i=0;i<4097;i++)diagnostics[i]='e';
if(write(2,diagnostics,e)!=e)return 7;
while(n){int k=n;if(k>65536)k=65536;if(write(1,b,k)!=k)return 8;n-=k;}}
`)
	compileDone()
	verdict, classifyErr := execution.Classify(o, true)
	if err != nil || classifyErr != nil || verdict != "AC" || o.Artifact == "" {
		t.Fatal("trusted boundary probe compilation", execution.FailureDetails(err))
	}
	pattern := make([]byte, 256)
	for i := range pattern {
		pattern[i] = byte(i)
	}
	want := bytes.Repeat(pattern, (8<<20)/len(pattern))
	for _, q := range []struct{ stdout, stderr, input int }{
		{8 << 20, 4096, 16 << 20},
		{(8 << 20) + 1, 4096, 0},
		{1, 4097, 0},
	} {
		input := []byte(strconv.Itoa(q.stdout) + " " + strconv.Itoa(q.stderr) + " " + strconv.Itoa(q.input) + "\n")
		if q.input != 0 {
			// The complete input, including the small header, stays within 16MiB.
			q.input -= len(input)
			header := []byte(strconv.Itoa(q.stdout) + " " + strconv.Itoa(q.stderr) + " " + strconv.Itoa(q.input) + "\n")
			if q.input+len(header) != 16<<20 {
				t.Fatal("trusted boundary input framing changed")
			}
			input = append(header, bytes.Repeat(pattern, (q.input+255)/256)[:q.input]...)
		}
		wire.last = staticRangeSumWireEvidence{}
		runCtx, runDone := context.WithTimeout(ctx, time.Duration(c.Runtime.Run.Wall)+5*time.Second)
		out, err := g.Run(runCtx, c.Runtime, o.Artifact, input)
		runDone()
		actual := wire.last
		if !actual.completeUsage || !actual.onlyStdoutID || !actual.onlyStderrInline || !actual.stderrAllE || actual.stderrBytes != q.stderr {
			t.Fatal("incomplete trusted wire evidence")
		}
		if q.stderr > 4096 {
			if !staticRangeSumExpectedStderrFailure(err) || !actual.ole || !actual.errorLabelMatches || actual.exit != 0 || actual.fileErrors != 1 || !actual.stderrCollector || actual.stdoutCollector || files.lastSize != 1 || files.lastSHA != sha256.Sum256([]byte{0}) {
				t.Fatal("stderr collector did not retain strict rejection", execution.FailureDetails(err))
			}
			t.Log("stderr4097 collector evidence rejected; output byte read and known cleanup completed")
			continue
		}
		verdict, classifyErr := execution.Classify(out, false)
		if err != nil || classifyErr != nil || !bytes.Equal(out.Stdout, want) || actual.errorPresent || actual.fileErrors != 0 || out.OutputTruncated {
			t.Fatal("raw boundary bytes or complete evidence", execution.FailureDetails(err))
		}
		if q.stdout == 8<<20 {
			if verdict != "AC" || !actual.accepted || actual.exit != 0 || !files.inputSample || !files.outputSample {
				t.Fatal("exact stdout8MiB/stderr4KiB/input16MiB boundary")
			}
		} else if verdict != "OLE" || !actual.ole || actual.exit != 25 || out.Exit != 25 {
			t.Fatal("SIGXFSZ boundary lacks complete narrow evidence")
		}
		t.Logf("raw requested=%d returned=%d SHA256=%x stderr=%d verdict=%s exit=%d CPU=%d memory=%d", q.stdout, len(out.Stdout), sha256.Sum256(out.Stdout), actual.stderrBytes, verdict, out.Exit, out.CPU, out.Memory)
	}
	cleanupCtx, cleanupDone := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupDone()
	if err := g.Remove(cleanupCtx, o.Artifact); err != nil {
		t.Fatal("trusted executable cleanup", execution.FailureDetails(err))
	}
	referenceStorageSample(t, h, "static-range-sum-after-cleanup")
}

type staticRangeSumWireEvidence struct {
	accepted, ole, errorPresent, completeUsage   bool
	errorLabelMatches                            bool
	onlyStdoutID, onlyStderrInline               bool
	stderrAllE, stdoutCollector, stderrCollector bool
	exit, stderrBytes, fileErrors                int
}

type staticRangeSumProbeTransport struct {
	host http.RoundTripper
	last staticRangeSumWireEvidence
}

func (p *staticRangeSumProbeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := p.host.RoundTrip(req)
	if err != nil || req.URL.Path != "/run" {
		return resp, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, (128<<10)+1))
	closeErr := resp.Body.Close()
	if readErr != nil || closeErr != nil || len(raw) > 128<<10 {
		return nil, errors.New("probe response unavailable")
	}
	var rows []struct {
		Status     string             `json:"status"`
		Error      string             `json:"error"`
		Exit       *int               `json:"exitStatus"`
		CPU        *int64             `json:"time"`
		Memory     *int64             `json:"memory"`
		Files      map[string]*string `json:"files"`
		IDs        map[string]string  `json:"fileIds"`
		FileErrors []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"fileError"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil || len(rows) != 1 {
		return nil, errors.New("probe response invalid")
	}
	r := rows[0]
	e := staticRangeSumWireEvidence{accepted: r.Status == "Accepted", ole: r.Status == "Output Limit Exceeded", errorPresent: r.Error != "", errorLabelMatches: r.Error == "Output Limit Exceeded", completeUsage: r.Exit != nil && r.CPU != nil && r.Memory != nil && *r.CPU >= 0 && *r.Memory >= 0, onlyStdoutID: len(r.IDs) == 1 && r.IDs["stdout"] != "", onlyStderrInline: len(r.Files) == 1 && r.Files["stderr"] != nil, fileErrors: len(r.FileErrors)}
	if r.Exit != nil {
		e.exit = *r.Exit
	}
	if stderr := r.Files["stderr"]; stderr != nil {
		e.stderrBytes, e.stderrAllE = len(*stderr), strings.Count(*stderr, "e") == len(*stderr)
	}
	for _, file := range r.FileErrors {
		e.stdoutCollector = e.stdoutCollector || file.Name == "stdout" && file.Type == "CollectSizeExceeded"
		e.stderrCollector = e.stderrCollector || file.Name == "stderr" && file.Type == "CollectSizeExceeded"
	}
	p.last = e
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	return resp, nil
}

type staticRangeSumMeasuredFiles struct {
	host                      *workerhost.Host
	t                         *testing.T
	inputSample, outputSample bool
	lastSize                  int
	lastSHA                   [32]byte
}

func (f *staticRangeSumMeasuredFiles) UploadFile(ctx context.Context, raw []byte) (string, error) {
	id, err := f.host.UploadFile(ctx, raw)
	if err == nil && len(raw) == 16<<20 {
		f.inputSample = true
		referenceStorageSample(f.t, f.host, "static-range-sum-input-live")
	}
	return id, err
}

func (f *staticRangeSumMeasuredFiles) ReadFile(ctx context.Context, id string, maximum int64) ([]byte, error) {
	raw, err := f.host.ReadFile(ctx, id, maximum)
	if err == nil && maximum == (8<<20)+1 {
		f.lastSize, f.lastSHA = len(raw), sha256.Sum256(raw)
		if len(raw) == 8<<20 && !f.outputSample {
			f.outputSample = true
			referenceStorageSample(f.t, f.host, "static-range-sum-output-live")
		}
	}
	return raw, err
}

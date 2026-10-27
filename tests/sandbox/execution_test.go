//go:build sandbox

package sandbox_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// dockerTransport is a test driver, not a deployed Worker transport. The REST
// port remains inside the offline container; all programs run inside go-judge.
type dockerTransport struct{ runs int }

func (d *dockerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != "127.0.0.1:5050" {
		return nil, fmt.Errorf("unexpected host")
	}
	if r.URL.Path == "/run" {
		d.runs++
	}
	args := []string{"--context", "colima-oj-m0", "exec", "-i", container, "curl", "-fsS", "--max-time", "20", "-X", r.Method, "-H", "Content-Type: application/json"}
	if r.Body != nil {
		args = append(args, "--data-binary", "@-")
	}
	args = append(args, r.URL.String())
	cmd := exec.CommandContext(r.Context(), "docker", args...)
	cmd.Stdin = r.Body
	// curl is capped as well as the adapter, so malformed output cannot grow the
	// test driver's buffer without bound before the adapter checks its response.
	var out cappedBuffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(out.Bytes()))}, nil
}

type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 128*1024+1 {
		return 0, fmt.Errorf("response too large")
	}
	return b.Buffer.Write(p)
}

func TestBoundedExecution(t *testing.T) {
	verifyEnvironment(t)
	hash := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	v, m, _ := experiment.SelectFixture(experiment.PinnedVersion)
	blobs := map[string][]byte{}
	for _, s := range []string{"1 2\n", "3\n", "-7 4\n", "-3\n"} {
		blobs[hash([]byte(s))] = []byte(s)
	}
	for _, tc := range []struct {
		name, source, verdict string
		points                int
	}{
		{"AC", `#include <iostream>
int main(){long long a,b;std::cin>>a>>b;std::cout<<a+b;}`, "AC", 2},
		{"first WA", `#include <cstdio>
int main(){puts("wrong");}`, "WA", 1},
		{"last WA", `#include <cstdio>
int main(){puts("3");}`, "WA", 2},
		{"CE", `this is not C++`, "CE", 0},
		{"RE", `int main(){return 1;}`, "RE", 1},
		{"TLE", `int main(){for(;;){asm volatile("");}}`, "TLE", 1},
		{"MLE", `#include <cstdlib>
int main(){volatile char*p=(char*)malloc(128*1024*1024);if(!p)return 2;for(long i=0;i<128*1024*1024;i+=4096)p[i]=1;for(;;){asm volatile("");}}`, "MLE", 1},
		{"OLE", `#include <cstdio>
int main(){for(;;)puts("unbounded output fixture");}`, "OLE", 1},
		{"invalid UTF8 output", `#include <cstdio>
int main(){for(int i=0;i<4096;i++)putchar(255);}`, "WA", 1},
		{"fresh filesystem", `#include <cstdio>
int main(){if(fopen("/tmp/previous-point","r"))return 1;FILE*f=fopen("/tmp/previous-point","w");if(!f)return 2;fclose(f);int a,b;scanf("%d%d",&a,&b);printf("%d",a+b);}`, "AC", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &dockerTransport{}
			sandbox := execution.NewGoJudge(transport, func(context.Context, experiment.ExecutionProfile) error { verifyEnvironment(t); return nil })
			d := materials.Descriptor{TaskMaterials: submission.TaskMaterials{Version: v, Language: "gnu++17", Manifest: m, ManifestSHA256: hash(m), Source: tc.source}, SourceSHA256: hash([]byte(tc.source))}
			r, err := execution.New(sandbox).Judge(context.Background(), d, blobs)
			if err != nil || r.Verdict != tc.verdict {
				t.Fatalf("verdict=%s compile=%+v error=%v", r.Verdict, r.Compile, err)
			}
			if transport.runs != 1+tc.points {
				t.Fatalf("expected one compile and %d tests; got %d requests", tc.points, transport.runs)
			}
			for _, p := range r.Points[tc.points:] {
				if p.Verdict != "NOT_RUN" || p.Usage != nil {
					t.Fatalf("invented point: %+v", p)
				}
			}
			var cache map[string]string
			if err := json.Unmarshal(api(t, "GET", "/file", nil), &cache); err != nil || len(cache) != 0 {
				t.Fatalf("artifact cleanup: entries=%d error=%v", len(cache), err)
			}
			if tc.verdict == "OLE" && !r.Points[0].OutputTruncated {
				t.Fatal("output truncation not recorded")
			}
			t.Logf("%s: points=%d max_usage=%+v", r.Verdict, tc.points, r.Max)
		})
	}
}

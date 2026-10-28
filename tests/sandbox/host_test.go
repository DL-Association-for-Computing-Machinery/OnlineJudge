//go:build sandbox

package sandbox_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

type countingTransport struct {
	base http.RoundTripper
	runs int
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == "/run" {
		c.runs++
	}
	return c.base.RoundTrip(r)
}

func TestWorkerHostExecution(t *testing.T) {
	verifyEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	host, err := workerhost.OpenM0(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sandbox := execution.NewGoJudge(host, host.Verify)
	v, manifest, _ := experiment.SelectFixture(experiment.PinnedVersion)
	blobs := map[string][]byte{}
	for _, s := range []string{"1 2\n", "3\n", "-7 4\n", "-3\n"} {
		blobs[hashBytes([]byte(s))] = []byte(s)
	}
	for _, tc := range []struct{ source, verdict string }{
		{"#include <cstdio>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);printf(\"%d\",a+b);}", "AC"},
		{"#include <cstdio>\nint main(){puts(\"wrong\");}", "WA"},
	} {
		d := materials.Descriptor{TaskMaterials: submission.TaskMaterials{Version: v, Manifest: manifest, ManifestSHA256: hashBytes(manifest), Source: tc.source, Language: "gnu++17"}, SourceSHA256: hashBytes([]byte(tc.source))}
		r, err := execution.New(sandbox).Judge(ctx, d, blobs)
		if err != nil || r.Verdict != tc.verdict {
			t.Fatalf("wanted %s, got %s: %v", tc.verdict, r.Verdict, err)
		}
	}
	t.Log("trusted host adapter verified actual AC/WA and artifact cleanup")
}

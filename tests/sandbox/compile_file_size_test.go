//go:build sandbox

package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// Record the actual response without resending a compile or changing evidence.
type compileSizeTape struct {
	transport dockerTransport
	results   []outcome
}

func (d *compileSizeTape) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := d.transport.RoundTrip(r)
	if err != nil || r.URL.Path != "/run" {
		return response, err
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 128*1024+2))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || len(data) > 128*1024+1 {
		return nil, errors.New("cannot observe bounded compile response")
	}
	var records []outcome
	if err := json.Unmarshal(data, &records); err != nil || len(records) != 1 {
		return nil, errors.New("invalid observed compile result count")
	}
	d.results = append(d.results, records[0])
	response.Body = io.NopCloser(bytes.NewReader(data))
	return response, nil
}

// A platform drift must not accidentally execute the deliberately oversized
// fixture. Compile is real; unexpected Run fails instead of fabricating CE.
type compileSizeSandbox struct {
	execution.Sandbox
	compileOnly bool
	runCalls    int
}

func (s *compileSizeSandbox) Run(ctx context.Context, p experiment.ExecutionProfile, id string, input []byte) (execution.Observation, error) {
	s.runCalls++
	if s.compileOnly {
		return execution.Observation{}, errors.New("oversized compile fixture unexpectedly reached Run")
	}
	return s.Sandbox.Run(ctx, p, id, input)
}

func testCompileFileSize(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	version, manifest, err := experiment.SelectFixture(experiment.PinnedVersion)
	if err != nil {
		t.Fatal(err)
	}
	blobs := map[string][]byte{}
	for _, value := range []string{"1 2\n", "3\n", "-7 4\n", "-3\n"} {
		blobs[hashBytes([]byte(value))] = []byte(value)
	}
	ordinary := "#include <cstdio>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);printf(\"%d\",a+b);}\n"
	source := func(size int) string {
		return fmt.Sprintf("asm(\".pushsection .rodata\\n.fill %d,1,1\\n.popsection\\n\");\n", size) + ordinary
	}
	descriptor := func(code string) materials.Descriptor {
		return materials.Descriptor{TaskMaterials: submission.TaskMaterials{Version: version, Language: "gnu++17", Manifest: manifest, ManifestSHA256: hashBytes(manifest), Source: code}, SourceSHA256: hashBytes([]byte(code))}
	}
	tape := &compileSizeTape{}
	sandbox := &compileSizeSandbox{Sandbox: execution.NewGoJudge(tape, func(context.Context, experiment.ExecutionProfile) error {
		verifyEnvironment(t)
		return nil
	})}
	runner := execution.New(sandbox)
	empty := func() {
		t.Helper()
		var cache map[string]string
		if err := json.Unmarshal(api(t, "GET", "/file", nil), &cache); err != nil || len(cache) != 0 {
			t.Fatal("compile file-size fixture left cached artifacts")
		}
	}
	control, err := runner.Judge(ctx, descriptor(source(256<<10)), blobs)
	if err != nil || control.Verdict != "AC" || control.Compile == nil || tape.transport.runs != 3 || sandbox.runCalls != 2 {
		t.Fatalf("finite assembly control failed: verdict=%s error=%v", control.Verdict, err)
	}
	empty()
	sandbox.compileOnly = true
	before := len(tape.results)
	failed, err := runner.Judge(ctx, descriptor(source((1<<20)+(128<<10))), blobs)
	if len(tape.results) <= before {
		t.Fatal("oversized fixture produced no structured compile observation")
	}
	raw := tape.results[before]
	diagnosticText := raw.Files["stderr"]
	if len(diagnosticText) > 512 {
		diagnosticText = diagnosticText[:512]
	}
	t.Logf("bounded fixed-compiler diagnostic: %q", diagnosticText)
	diagnostic := strings.Contains(raw.Files["stderr"], "File size limit exceeded signal terminated program as")
	t.Logf("fixed g++ compile: raw_status=%q exit=%d cpu=%d memory=%d file_errors=%d artifacts=%d compiler_reports_file_size=%t stderr_sha256=%s; runner=%s adapter_error=%v",
		raw.Status, raw.Exit, raw.Time, raw.Memory, len(raw.FileErrors), len(raw.IDs), diagnostic, hashBytes([]byte(raw.Files["stderr"])), failed.Verdict, err)
	if len(tape.results) != before+1 || sandbox.runCalls != 2 {
		t.Fatal("oversized fixture did not stop after its single real Compile")
	}
	// The text is an auxiliary fixed-compiler diagnostic, never the verdict
	// oracle. Unknown adapter evidence must remain SE and stop this exploration.
	if err != nil || failed.Verdict != "CE" || failed.Compile == nil {
		t.Fatal("fixed compiler did not yield a supported CE; retain Runner isolation and inspect evidence")
	}
	p := experiment.PinnedEnvironment()
	if raw.Status != "Nonzero Exit Status" || raw.Exit != 4 || raw.Error != "" || len(raw.FileErrors) != 0 || len(raw.IDs) != 0 ||
		!diagnostic || raw.Time <= 0 || raw.Time >= p.Compile.CPU || raw.Memory <= 0 || raw.Memory >= p.Compile.Memory || failed.Compile.OutputTruncated || failed.Max != nil {
		t.Fatal("compiler file-size evidence differs from the targeted finite nonzero-exit path")
	}
	for _, point := range failed.Points {
		if point.Verdict != "NOT_RUN" || point.Usage != nil {
			t.Fatal("compile failure invented point execution")
		}
	}
	empty()
	// Reuse is required only after the real, supported CE path above. An SE
	// would have returned already, preserving the Runner's permanent stop rule.
	sandbox.compileOnly = false
	resumed, err := runner.Judge(ctx, descriptor(ordinary), blobs)
	if err != nil || resumed.Verdict != "AC" || tape.transport.runs != 7 || sandbox.runCalls != 4 {
		t.Fatalf("same Runner after compiler CE: verdict=%s error=%v", resumed.Verdict, err)
	}
	empty()
	t.Log("256KiB control AC → bounded oversized Compile CE/zero Run → ordinary AC on the same Runner; unchanged profile and empty artifact cache")
}

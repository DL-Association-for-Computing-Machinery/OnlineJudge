package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func associativeAdapter(t *testing.T) (*CachedGoJudge, *cacheFixture, experiment.ExecutionProfile) {
	t.Helper()
	f := &cacheFixture{t: t, data: map[string][]byte{}, reference: true, referenceOutput: 8 << 20}
	g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f, experiment.ReferenceAssociativeArray)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Verify(context.Background(), g.runtime); err != nil {
		t.Fatal(err)
	}
	return g, f, g.runtime
}

func TestAssociativeArrayCompleteTwentyPointPayloadAndCleanup(t *testing.T) {
	ctx := context.Background()
	g, f, p := associativeAdapter(t)
	o, err := g.Compile(ctx, p, string(bytes.Repeat([]byte{'s'}, 474)))
	if err != nil {
		t.Fatal(err)
	}
	f.stdout = bytes.Repeat([]byte{'x'}, 8<<20)
	remaining := 412885310
	for i := 0; i < 20; i++ {
		size := remaining / (20 - i)
		if i == 0 {
			size = 39588095
		}
		remaining -= size
		out, err := g.Run(ctx, p, o.Artifact, bytes.Repeat([]byte{'i'}, size))
		if err != nil || !bytes.Equal(out.Stdout, f.stdout) || len(f.data) != 1 || g.liveObjects != 1 {
			t.Fatal("complete output or per-point cleanup", i, err)
		}
	}
	if remaining != 0 || g.totalBytes != 993543748 || g.totalBytes <= 512<<20 || g.totalBytes >= 1<<30 {
		t.Fatal("complete-source payload envelope")
	}
	if err := g.Remove(ctx, o.Artifact); err != nil {
		t.Fatal(err)
	}
	if f.runs != 20 || g.operations != 127 || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
		t.Fatal("complete-source operation envelope or final cleanup")
	}
}

func TestAssociativeArrayExactInputOutputReservationsAndWireBudgets(t *testing.T) {
	ctx := context.Background()
	g, f, p := associativeAdapter(t)
	measured := &staticRangeSumPeakFiles{base: f, adapter: g}
	g.files = measured
	commands := 0
	g.control.client.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/run" {
			raw, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			if err := req.Body.Close(); err != nil {
				t.Fatal(err)
			}
			req.Body = io.NopCloser(bytes.NewReader(raw))
			var wire struct {
				Cmd []map[string]any `json:"cmd"`
			}
			if err := json.Unmarshal(raw, &wire); err != nil || len(wire.Cmd) != 1 {
				t.Fatal("invalid request", err)
			}
			c, limits := wire.Cmd[0], p.Run
			if len(c["args"].([]any)) > 1 {
				limits = p.Compile
			}
			if c["cpuLimit"] != float64(limits.CPU) || c["clockLimit"] != float64(limits.Wall) || c["memoryLimit"] != float64(limits.Memory) || c["procLimit"] != float64(limits.Processes) {
				t.Fatal("new runtime limits were not sent to executor")
			}
			commands++
		}
		return f.RoundTrip(req)
	})
	o, err := g.Compile(ctx, p, "source")
	if err != nil {
		t.Fatal(err)
	}
	pattern := make([]byte, 256)
	for i := range pattern {
		pattern[i] = byte(i)
	}
	f.stdout = bytes.Repeat(pattern, (8<<20)/len(pattern))
	out, err := g.Run(ctx, p, o.Artifact, bytes.Repeat(pattern, (48<<20)/len(pattern)))
	verdict, classifyErr := Classify(out, false)
	if err != nil || classifyErr != nil || verdict != "AC" || !bytes.Equal(out.Stdout, f.stdout) || out.OutputTruncated || commands != 2 {
		t.Fatal("exact input/output bytes or runtime wire", err, classifyErr)
	}
	if measured.peakBytes != (57<<20)+1 || measured.peakObjects != 3 || measured.stdoutReads != 1 || g.liveBytes != 1<<20 || g.liveObjects != 1 {
		t.Fatal("large input/output/executable reservations or point cleanup")
	}
	if err := g.Remove(ctx, o.Artifact); err != nil || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
		t.Fatal("exact boundary executable cleanup", err)
	}
}

func TestAssociativeArrayPayloadCeilingRetainsOwnedCleanup(t *testing.T) {
	ctx := context.Background()
	g, f, p := associativeAdapter(t)
	o, err := g.Compile(ctx, p, "source")
	if err != nil {
		t.Fatal(err)
	}
	g.totalBytes = (1 << 30) - 1
	uploads := f.uploads
	_, err = g.Run(ctx, p, o.Artifact, []byte{'x'})
	if err == nil || g.totalBytes != 1<<30 || f.uploads != uploads+1 || f.runs != 0 || g.liveObjects != 1 || len(f.data) != 1 {
		t.Fatal("payload ceiling did not reject readback and clean known input", err)
	}
	if err := g.Remove(ctx, o.Artifact); err != nil || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
		t.Fatal("payload exhaustion blocked known executable removal", err)
	}
}

func TestAssociativeArrayFileCeilingRejectsBeforeInputUpload(t *testing.T) {
	ctx := context.Background()
	g, f, p := associativeAdapter(t)
	o, err := g.Compile(ctx, p, "source")
	if err != nil {
		t.Fatal(err)
	}
	uploads := f.uploads
	_, err = g.Run(ctx, p, o.Artifact, bytes.Repeat([]byte{'i'}, (48<<20)+1))
	if err == nil || f.uploads != uploads || f.runs != 0 || g.liveObjects != 1 || len(f.data) != 1 {
		t.Fatal("oversized input reached upload or lost executable ownership")
	}
	if err := g.Remove(ctx, o.Artifact); err != nil || g.liveBytes != 0 || len(f.data) != 0 {
		t.Fatal("file ceiling blocked owned cleanup", err)
	}
}

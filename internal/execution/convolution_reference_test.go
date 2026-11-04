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

func convolutionAdapter(t *testing.T) (*CachedGoJudge, *cacheFixture, experiment.ExecutionProfile) {
	t.Helper()
	f := &cacheFixture{t: t, data: map[string][]byte{}, reference: true, referenceOutput: 16 << 20}
	g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f, experiment.ReferenceConvolution)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Verify(context.Background(), g.runtime); err != nil {
		t.Fatal(err)
	}
	return g, f, g.runtime
}

func TestConvolutionCompleteFiftyThreePointTransferAndCleanup(t *testing.T) {
	ctx := context.Background()
	g, f, p := convolutionAdapter(t)
	o, err := g.Compile(ctx, p, string(bytes.Repeat([]byte{'s'}, 3284)))
	if err != nil {
		t.Fatal(err)
	}
	f.stdout = bytes.Repeat([]byte{'x'}, 16<<20)
	remaining := 211547577
	for i := 0; i < 53; i++ {
		size := remaining / (53 - i)
		if i == 0 {
			size = 10485774
		}
		remaining -= size
		out, err := g.Run(ctx, p, o.Artifact, bytes.Repeat([]byte{'i'}, size))
		if err != nil || !bytes.Equal(out.Stdout, f.stdout) || len(f.data) != 1 || g.liveObjects != 1 {
			t.Fatal("complete source transfer or point cleanup", i, err)
		}
	}
	if remaining != 0 || g.totalBytes != 1312294223 || g.totalBytes <= 1<<30 || g.totalBytes >= 2<<30 {
		t.Fatal("complete source conservative payload envelope")
	}
	if err := g.Remove(ctx, o.Artifact); err != nil || f.runs != 53 || g.operations != 325 || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
		t.Fatal("complete source operation envelope or final cleanup", err)
	}
}

func TestConvolutionExactRawBytesReservationsAndWireLimits(t *testing.T) {
	for _, signal := range []bool{false, true} {
		g, f, p := convolutionAdapter(t)
		measured := &staticRangeSumPeakFiles{base: f, adapter: g}
		g.files = measured
		runs := 0
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
					t.Fatal("invalid wire request", err)
				}
				c := wire.Cmd[0]
				if len(c["args"].([]any)) == 1 {
					runs++
					if c["cpuLimit"] != float64(p.Run.CPU) || c["clockLimit"] != float64(p.Run.Wall) || c["memoryLimit"] != float64(p.Run.Memory) || c["procLimit"] != float64(p.Run.Processes) || c["copyOutMax"] != float64((16<<20)+1) {
						t.Fatal("convolution execution or raw output limits lost on wire")
					}
				}
			}
			return f.RoundTrip(req)
		})
		o, err := g.Compile(context.Background(), p, "source")
		if err != nil {
			t.Fatal(err)
		}
		pattern := make([]byte, 256)
		for i := range pattern {
			pattern[i] = byte(i)
		}
		f.stdout = bytes.Repeat(pattern, (16<<20)/len(pattern))
		if signal {
			f.fail = "sigxfsz"
		}
		out, err := g.Run(context.Background(), p, o.Artifact, bytes.Repeat(pattern, (16<<20)/len(pattern)))
		verdict, classifyErr := Classify(out, false)
		want := "AC"
		if signal {
			want = "OLE"
		}
		if err != nil || classifyErr != nil || verdict != want || runs != 1 || !bytes.Equal(out.Stdout, f.stdout) || out.OutputTruncated || signal && out.Exit != 25 {
			t.Fatal("exact raw bytes or narrow SIGXFSZ evidence", err, classifyErr)
		}
		if measured.peakBytes != (33<<20)+1 || measured.peakObjects != 3 || measured.stdoutReads != 1 || g.liveBytes != 1<<20 || g.liveObjects != 1 {
			t.Fatal("input/stdout/executable reservations or cleanup")
		}
		if err := g.Remove(context.Background(), o.Artifact); err != nil || g.liveBytes != 0 || len(f.data) != 0 {
			t.Fatal("exact boundary cleanup", err)
		}
	}
}

func TestConvolutionTwoGiBExactCapAndOwnedCleanup(t *testing.T) {
	for _, mode := range []string{"exact-download", "readback-plus-one"} {
		t.Run(mode, func(t *testing.T) {
			g, f, p := convolutionAdapter(t)
			o, err := g.Compile(context.Background(), p, "source")
			if err != nil {
				t.Fatal(err)
			}
			// Precharge counters; this does not claim an actual 2GiB transfer.
			if mode == "exact-download" {
				g.totalBytes = (2 << 30) - int64(p.OutputBytes) - 1
				if _, err := g.Run(context.Background(), p, o.Artifact, nil); err != nil || g.totalBytes != 2<<30 {
					t.Fatal("exact payload ceiling rejected", err)
				}
				if _, err := g.Run(context.Background(), p, o.Artifact, []byte{'i'}); err == nil || f.runs != 1 {
					t.Fatal("payload plus one accepted")
				}
			} else {
				g.totalBytes = (2 << 30) - 1
				uploads := f.uploads
				if _, err := g.Run(context.Background(), p, o.Artifact, []byte{'i'}); err == nil || f.uploads != uploads+1 || f.runs != 0 || g.totalBytes != 2<<30 {
					t.Fatal("over-budget readback dispatched", err)
				}
			}
			if g.liveObjects != 1 || len(f.data) != 1 {
				t.Fatal("payload exhaustion lost point cleanup")
			}
			if err := g.Remove(context.Background(), o.Artifact); err != nil || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
				t.Fatal("payload exhaustion blocked owned executable removal", err)
			}
		})
	}
}

func TestConvolutionLargeGenericInputFailsBeforeExecution(t *testing.T) {
	g, f, p := convolutionAdapter(t)
	o, err := g.Compile(context.Background(), p, "source")
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.Run(context.Background(), p, o.Artifact, bytes.Repeat([]byte{'i'}, 48<<20))
	d := FailureDetails(err)
	if err == nil || f.runs != 0 || d == nil || len(d.Entries) != 1 || d.Entries[0].Phase != "control" || d.Entries[0].Role != "stdout" || d.Entries[0].Code != "budget" || g.liveBytes != 1<<20 || g.liveObjects != 1 || len(f.data) != 1 {
		t.Fatal("48MiB input plus stdout reservation exceeded live cap or leaked input", err)
	}
	if err := g.Remove(context.Background(), o.Artifact); err != nil || g.liveBytes != 0 || len(f.data) != 0 {
		t.Fatal("live ceiling blocked owned cleanup", err)
	}
}

func TestConvolutionRejectsAllOldRuntimeIdentities(t *testing.T) {
	for _, oldTier := range []experiment.ReferenceTier{experiment.ReferenceSmall, experiment.ReferenceMedium, experiment.ReferenceMediumBulk, experiment.ReferencePrimality, experiment.ReferenceStaticRMQ, experiment.ReferenceStaticRangeSum, experiment.ReferenceAssociativeArray} {
		for _, pair := range [][2]experiment.ReferenceTier{{experiment.ReferenceConvolution, oldTier}, {oldTier, experiment.ReferenceConvolution}} {
			f := &cacheFixture{t: t, data: map[string][]byte{}}
			g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error {
				t.Fatal("mismatched runtime reached environment Verify")
				return nil
			}, f, pair[0])
			if err != nil {
				t.Fatal(err)
			}
			other, err := experiment.ResolveReferenceTier(pair[1])
			if err != nil {
				t.Fatal(err)
			}
			if err := g.Verify(context.Background(), other.Runtime); err == nil || f.compiles != 0 || f.uploads != 0 {
				t.Fatal("convolution borrowed another tier runtime")
			}
		}
	}
}

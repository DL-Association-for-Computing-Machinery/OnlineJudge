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

func staticRangeSumAdapter(t *testing.T) (*CachedGoJudge, *cacheFixture, experiment.ExecutionProfile) {
	t.Helper()
	f := &cacheFixture{t: t, data: map[string][]byte{}, reference: true, referenceOutput: 8 << 20}
	g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f, experiment.ReferenceStaticRangeSum)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Verify(context.Background(), g.runtime); err != nil {
		t.Fatal(err)
	}
	return g, f, g.runtime
}

func TestStaticRangeSumCompleteElevenPointWorstPayload(t *testing.T) {
	ctx := context.Background()
	g, f, p := staticRangeSumAdapter(t)
	o, err := g.Compile(ctx, p, string(bytes.Repeat([]byte{'s'}, 328)))
	if err != nil {
		t.Fatal(err)
	}
	f.stdout = bytes.Repeat([]byte{'x'}, 8<<20)
	remaining := 91745127
	for i := 0; i < 11; i++ {
		size := remaining / (11 - i)
		if i == 0 {
			size = 11722894
		}
		remaining -= size
		out, err := g.Run(ctx, p, o.Artifact, bytes.Repeat([]byte{'i'}, size))
		if err != nil || !bytes.Equal(out.Stdout, f.stdout) || len(f.data) != 1 || g.liveObjects != 1 {
			t.Fatal("complete raw output or per-point cleanup", i, err)
		}
	}
	if remaining != 0 || g.totalBytes != 275765609 || g.totalBytes <= 256<<20 || g.totalBytes >= 512<<20 {
		t.Fatal("whole-source envelope was not conservatively charged")
	}
	if err := g.Remove(ctx, o.Artifact); err != nil {
		t.Fatal(err)
	}
	if f.runs != 11 || g.operations != 73 || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
		t.Fatal("complete source operation envelope or final cleanup")
	}
}

func TestStaticRangeSumPayloadExhaustionStillAllowsOwnedCleanup(t *testing.T) {
	ctx := context.Background()
	g, f, p := staticRangeSumAdapter(t)
	o, err := g.Compile(ctx, p, "source")
	if err != nil {
		t.Fatal(err)
	}
	g.totalBytes = (512 << 20) - 1
	uploads := f.uploads
	_, err = g.Run(ctx, p, o.Artifact, []byte{'x'})
	if err == nil || g.totalBytes != 512<<20 || f.uploads != uploads+1 || f.runs != 0 || g.liveObjects != 1 || len(f.data) != 1 {
		t.Fatal("readback exceeded cap or lost known cleanup", err)
	}
	if err := g.Remove(ctx, o.Artifact); err != nil || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
		t.Fatal("payload exhaustion blocked owned executable deletion", err)
	}
}

func TestStaticRangeSumRuntimeCannotBeBorrowedByExistingTiers(t *testing.T) {
	tiers := []experiment.ReferenceTier{experiment.ReferenceSmall, experiment.ReferenceMedium, experiment.ReferencePrimality, experiment.ReferenceStaticRangeSum, experiment.ReferenceAssociativeArray}
	for _, tier := range tiers {
		for _, other := range tiers {
			if tier == other {
				continue
			}
			f := &cacheFixture{t: t, data: map[string][]byte{}}
			g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error {
				t.Fatal("mismatched runtime reached environment verify")
				return nil
			}, f, tier)
			if err != nil {
				t.Fatal(err)
			}
			c, err := experiment.ResolveReferenceTier(other)
			if err != nil {
				t.Fatal(err)
			}
			if err := g.Verify(context.Background(), c.Runtime); err == nil || f.compiles != 0 || f.uploads != 0 {
				t.Fatal("runtime identity widened")
			}
		}
	}
}

type staticRangeSumPeakFiles struct {
	base        *cacheFixture
	adapter     *CachedGoJudge
	peakBytes   int64
	peakObjects int
	stdoutReads int
}

func (f *staticRangeSumPeakFiles) sample() {
	if f.adapter.liveBytes > f.peakBytes {
		f.peakBytes = f.adapter.liveBytes
	}
	if f.adapter.liveObjects > f.peakObjects {
		f.peakObjects = f.adapter.liveObjects
	}
}

func (f *staticRangeSumPeakFiles) UploadFile(ctx context.Context, raw []byte) (string, error) {
	f.sample()
	return f.base.UploadFile(ctx, raw)
}

func (f *staticRangeSumPeakFiles) ReadFile(ctx context.Context, id string, maximum int64) ([]byte, error) {
	f.sample()
	if object := f.adapter.objects[id]; object != nil && object.role == "stdout" {
		f.stdoutReads++
	}
	return f.base.ReadFile(ctx, id, maximum)
}

func TestStaticRangeSumExactRawOutputAndLiveReservations(t *testing.T) {
	for _, signal := range []bool{false, true} {
		g, f, p := staticRangeSumAdapter(t)
		measured := &staticRangeSumPeakFiles{base: f, adapter: g}
		g.files = measured
		o, err := g.Compile(context.Background(), p, "source")
		if err != nil {
			t.Fatal(err)
		}
		pattern := make([]byte, 256)
		for i := range pattern {
			pattern[i] = byte(i)
		}
		f.stdout = bytes.Repeat(pattern, (8<<20)/len(pattern))
		if signal {
			f.fail = "sigxfsz"
		}
		out, err := g.Run(context.Background(), p, o.Artifact, bytes.Repeat(pattern, (16<<20)/len(pattern)))
		verdict, classifyErr := Classify(out, false)
		want := "AC"
		if signal {
			want = "OLE"
		}
		if err != nil || classifyErr != nil || verdict != want || !bytes.Equal(out.Stdout, f.stdout) || out.OutputTruncated || signal && out.Exit != 25 {
			t.Fatal("exact raw output or narrow SIGXFSZ evidence", err, classifyErr)
		}
		if measured.peakBytes != (25<<20)+1 || measured.peakObjects != 3 || measured.stdoutReads != 1 || g.liveBytes != 1<<20 || g.liveObjects != 1 {
			t.Fatal("input/stdout/executable reservations or point cleanup")
		}
		if err := g.Remove(context.Background(), o.Artifact); err != nil || g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 {
			t.Fatal("raw boundary cleanup", err)
		}
	}
}

func TestStaticRangeSumSignalEvidenceRemainsNarrow(t *testing.T) {
	c, err := experiment.ResolveReferenceTier(experiment.ReferenceStaticRangeSum)
	if err != nil {
		t.Fatal(err)
	}
	base := func() goJudgeResult {
		exit, cpu, memory := 25, int64(1), int64(1)
		out, stderr := string(bytes.Repeat([]byte{'x'}, 8<<20)), ""
		return goJudgeResult{Status: "Output Limit Exceeded", Exit: &exit, CPU: &cpu, Memory: &memory, Files: map[string]*string{"stdout": &out, "stderr": &stderr}}
	}
	g := &CachedGoJudge{reference: true}
	for name, mutate := range map[string]func(*goJudgeResult){
		"error":          func(r *goJudgeResult) { r.Error = "SIGXFSZ" },
		"exit":           func(r *goJudgeResult) { *r.Exit = 0 },
		"missing-exit":   func(r *goJudgeResult) { r.Exit = nil },
		"missing-cpu":    func(r *goJudgeResult) { r.CPU = nil },
		"negative-cpu":   func(r *goJudgeResult) { *r.CPU = -1 },
		"missing-memory": func(r *goJudgeResult) { r.Memory = nil },
		"missing-stdout": func(r *goJudgeResult) { delete(r.Files, "stdout") },
		"missing-stderr": func(r *goJudgeResult) { delete(r.Files, "stderr") },
		"foreign-id":     func(r *goJudgeResult) { r.IDs = map[string]string{"main": "foreign"} },
	} {
		t.Run(name, func(t *testing.T) {
			r := base()
			mutate(&r)
			if _, err := g.observation(r, c.Runtime, false); err == nil {
				t.Fatal("contradictory output-limit evidence accepted")
			}
		})
	}
	if _, err := g.observation(base(), c.Runtime, true); err == nil {
		t.Fatal("run SIGXFSZ normalized during compilation")
	}
}

func TestStaticRangeSumExtraWireRoleRejectedBeforeStdoutAdoption(t *testing.T) {
	ctx := context.Background()
	g, f, p := staticRangeSumAdapter(t)
	measured := &staticRangeSumPeakFiles{base: f, adapter: g}
	g.files = measured
	o, err := g.Compile(ctx, p, "source")
	if err != nil {
		t.Fatal(err)
	}
	f.fail = "sigxfsz"
	g.control.client.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := f.RoundTrip(req)
		if err != nil || req.Method != "POST" || req.URL.Path != "/run" {
			return resp, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, (128<<10)+1))
		closeErr := resp.Body.Close()
		if readErr != nil || closeErr != nil || len(raw) > 128<<10 {
			t.Fatal("synthetic wire response unavailable", readErr, closeErr)
		}
		var rows []map[string]any
		if err := json.Unmarshal(raw, &rows); err != nil || len(rows) != 1 {
			t.Fatal("synthetic result unavailable", err)
		}
		rows[0]["files"].(map[string]any)["extra"] = ""
		raw, err = json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(raw)), nil
	})
	deletes := len(f.deletes)
	_, err = g.Run(ctx, p, o.Artifact, []byte("input"))
	details := FailureDetails(err)
	if err == nil || details == nil || details.Truncated || len(details.Entries) != 1 || details.Entries[0].Phase != "validate" || details.Entries[0].Role != "stdout" || details.Entries[0].Code != "ownership" || measured.stdoutReads != 0 {
		t.Fatal("extra wire role reached raw download or lost ownership classification")
	}
	if len(f.deletes) != deletes+1 || len(g.objects) != 1 || g.objects[o.Artifact] == nil {
		t.Fatal("known input cleanup or executable ownership lost")
	}
	runs, uploads := f.runs, f.uploads
	if _, err := g.Run(ctx, p, o.Artifact, nil); err == nil || f.runs != runs || f.uploads != uploads {
		t.Fatal("invalid receipt did not fence further execution")
	}
	if err := g.Remove(ctx, o.Artifact); err != nil || len(g.objects) != 0 || g.liveObjects != 1 || g.liveBytes != int64(p.OutputBytes+1) {
		t.Fatal("known executable cleanup or unknown stdout reservation lost", err)
	}
}

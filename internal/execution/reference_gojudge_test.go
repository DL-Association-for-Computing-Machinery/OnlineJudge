package execution

import (
	"bytes"
	"context"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func newReferenceFixture(t *testing.T) (*CachedGoJudge, *cacheFixture) {
	t.Helper()
	f := &cacheFixture{t: t, data: map[string][]byte{}, reference: true}
	g := NewReferenceGoJudge(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f)
	if err := g.Verify(context.Background(), experiment.PinnedReferenceEnvironment()); err != nil {
		t.Fatal(err)
	}
	return g, f
}

func TestReferenceRuntimeIdentityAndDiagnosticBoundaries(t *testing.T) {
	for _, reference := range []bool{false, true} {
		f := &cacheFixture{t: t, data: map[string][]byte{}}
		g := NewCachedGoJudge(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f)
		wrong := experiment.PinnedReferenceEnvironment()
		if reference {
			g = NewReferenceGoJudge(f, g.control.verify, f)
			wrong = experiment.PinnedEnvironment()
		}
		if err := g.Verify(context.Background(), wrong); err == nil {
			t.Fatal("cross-profile verification accepted")
		}
	}
	p := experiment.PinnedReferenceEnvironment()
	for _, fail := range []string{"compile-stdout", "compile-stderr", "run-stderr", "over", "OLE", "collector-OLE", "sigxfsz"} {
		t.Run(fail, func(t *testing.T) {
			g, f := newReferenceFixture(t)
			if fail == "compile-stdout" || fail == "compile-stderr" {
				f.fail = fail
			}
			o, err := g.Compile(context.Background(), p, "source")
			if f.fail != "" {
				if err == nil {
					t.Fatal("excess compile diagnostics accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			f.fail = fail
			out, err := g.Run(context.Background(), p, o.Artifact, []byte("input"))
			if fail == "OLE" || fail == "collector-OLE" || fail == "sigxfsz" {
				v, e := Classify(out, false)
				if err != nil || e != nil || v != "OLE" {
					t.Fatal("overflow evidence lost", err, e)
				}
			} else if err == nil {
				t.Fatal("excess raw output/diagnostic accepted")
			}
			if err := g.Remove(context.Background(), o.Artifact); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReferenceCollectorLabelNeedsCompleteOverflowEvidence(t *testing.T) {
	p := experiment.PinnedReferenceEnvironment()
	base := func() goJudgeResult {
		exit := 0
		cpu, memory := int64(1), int64(1)
		out, stderr := string(bytes.Repeat([]byte{'x'}, p.OutputBytes+1)), ""
		r := goJudgeResult{Status: "Output Limit Exceeded", Error: "Output Limit Exceeded", Exit: &exit, CPU: &cpu, Memory: &memory, Files: map[string]*string{"stdout": &out, "stderr": &stderr}}
		r.FileErrors = append(r.FileErrors, struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}{"stdout", "CollectSizeExceeded"})
		return r
	}
	g := &CachedGoJudge{reference: true}
	o, err := g.observation(base(), p, false)
	v, e := Classify(o, false)
	if err != nil || e != nil || v != "OLE" || !o.OutputTruncated {
		t.Fatal("pinned collector overflow rejected", err, e)
	}
	for name, mutate := range map[string]func(*goJudgeResult){
		"error":          func(r *goJudgeResult) { r.Error = "other executor failure" },
		"status":         func(r *goJudgeResult) { r.Status = "Accepted" },
		"exit":           func(r *goJudgeResult) { *r.Exit = 1 },
		"missing-exit":   func(r *goJudgeResult) { r.Exit = nil },
		"missing-cpu":    func(r *goJudgeResult) { r.CPU = nil },
		"negative-cpu":   func(r *goJudgeResult) { *r.CPU = -1 },
		"excess-cpu":     func(r *goJudgeResult) { *r.CPU = p.Run.CPU + 1 },
		"missing-memory": func(r *goJudgeResult) { r.Memory = nil },
		"excess-memory":  func(r *goJudgeResult) { *r.Memory = p.Run.Memory + 1 },
		"missing-stderr": func(r *goJudgeResult) { delete(r.Files, "stderr") },
		"excess-stderr": func(r *goJudgeResult) {
			s := string(bytes.Repeat([]byte{'e'}, 3*experiment.ReferenceDiagnosticBytes+1))
			r.Files["stderr"] = &s
		},
		"short-stdout":       func(r *goJudgeResult) { s := ""; r.Files["stdout"] = &s },
		"missing-file-error": func(r *goJudgeResult) { r.FileErrors = nil },
		"mixed-file-error":   func(r *goJudgeResult) { r.FileErrors = append(r.FileErrors, r.FileErrors[0]) },
		"stderr-role":        func(r *goJudgeResult) { r.FileErrors[0].Name = "stderr" },
		"other-file-error":   func(r *goJudgeResult) { r.FileErrors[0].Type = "CopyOutOpen" },
		"unexpected-id":      func(r *goJudgeResult) { r.IDs = map[string]string{"main": "foreign"} },
	} {
		t.Run(name, func(t *testing.T) {
			r := base()
			mutate(&r)
			if _, err := g.observation(r, p, false); err == nil {
				t.Fatal("contradictory collector evidence accepted")
			}
		})
	}
	if _, err := g.observation(base(), p, true); err == nil {
		t.Fatal("collector error normalized during compile")
	}
	m0 := experiment.PinnedEnvironment()
	r := base()
	small := string(bytes.Repeat([]byte{'x'}, m0.OutputBytes+1))
	r.Files["stdout"] = &small
	if _, err := (&CachedGoJudge{}).observation(r, m0, false); err == nil {
		t.Fatal("M0 error rules widened")
	}
}

func TestReferenceEighteenPointCacheLedgerAndExactOutputCap(t *testing.T) {
	g, f := newReferenceFixture(t)
	p := experiment.PinnedReferenceEnvironment()
	o, err := g.Compile(context.Background(), p, "source")
	if err != nil {
		t.Fatal(err)
	}
	f.stdout = bytes.Repeat([]byte{'x'}, p.OutputBytes)
	for i := 0; i < 18; i++ {
		out, err := g.Run(context.Background(), p, o.Artifact, bytes.Repeat([]byte{'i'}, 2<<20))
		if err != nil || !bytes.Equal(out.Stdout, f.stdout) || len(f.data) != 1 {
			t.Fatal("complete trial budget failed", i, err)
		}
	}
	if err := g.Remove(context.Background(), o.Artifact); err != nil {
		t.Fatal(err)
	}
	if g.liveBytes != 0 || g.liveObjects != 0 || len(f.data) != 0 || f.runs != 18 || g.operations >= g.limits.Operations || g.totalBytes >= g.limits.PayloadBytes {
		t.Fatal("trial quota or cleanup accounting")
	}
}

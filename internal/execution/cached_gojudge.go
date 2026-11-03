package execution

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

// FileTransport is the explicit bounded raw-file capability. Callers own an
// exclusive sandbox lifetime; cancellation or an unknown receipt requires Stop.
type FileTransport interface {
	UploadFile(context.Context, []byte) (string, error)
	ReadFile(context.Context, string, int64) ([]byte, error)
}

// CachedGoJudge uses private cached files for source, stdin and stdout. Its
// single-operation ledger is fenced after any failure; only owned cleanup
// remains permitted. The caller must independently confirm whole-sandbox Stop.
type CachedGoJudge struct {
	control                    *GoJudge
	files                      FileTransport
	limits                     experiment.ReferenceFilesProfile
	runtime                    experiment.ExecutionProfile
	reference                  bool
	busy                       atomic.Bool
	verified, compiled, failed bool
	executable                 string
	objects                    map[string]*cachedObject
	seen                       map[string]bool
	liveBytes, totalBytes      int64
	liveObjects, operations    int
}

// NewCachedGoJudge keeps the M0 constructor and transport subset unchanged.
// Only the separately pinned file experiment and fixed runtime are admitted.
func NewCachedGoJudge(transport http.RoundTripper, verify func(context.Context, experiment.ExecutionProfile) error, files FileTransport) *CachedGoJudge {
	return &CachedGoJudge{control: NewGoJudge(transport, verify), files: files, limits: experiment.PinnedReferenceFiles(), runtime: experiment.PinnedEnvironment(), objects: map[string]*cachedObject{}, seen: map[string]bool{}}
}

// NewReferenceGoJudge binds only the separately admitted unionfind runtime.
// It cannot widen the M0 constructor or accept arbitrary caller budgets.
func NewReferenceGoJudge(transport http.RoundTripper, verify func(context.Context, experiment.ExecutionProfile) error, files FileTransport) *CachedGoJudge {
	g := NewCachedGoJudge(transport, verify, files)
	g.runtime, g.reference = experiment.PinnedReferenceEnvironment(), true
	return g
}

// Verify admits one fixed runtime and probes the actual trusted host/executor.
func (g *CachedGoJudge) Verify(ctx context.Context, p experiment.ExecutionProfile) (err error) {
	if err = g.acquire(ctx, false); err != nil {
		return err
	}
	defer func() { err = g.finish(ctx, err) }()
	if g.verified || g.files == nil || !reflect.DeepEqual(p, g.runtime) {
		return errors.New("unsupported cached sandbox profile")
	}
	if err = g.step(0, false); err != nil {
		return err
	}
	if err = g.step(0, false); err != nil {
		return err
	}
	if err = g.control.Verify(ctx, p); err != nil {
		return cachedError(ctx, "cached sandbox verification failed", err)
	}
	g.verified = true
	return nil
}

// Compile verifies uploaded source bytes, compiles once, and retains only its
// owned executable. Source deletion is independently bounded even on failure.
func (g *CachedGoJudge) Compile(ctx context.Context, p experiment.ExecutionProfile, source string) (o Observation, err error) {
	if err = g.acquire(ctx, false); err != nil {
		return o, err
	}
	defer func() { err = g.finish(ctx, err) }()
	if !g.verified || g.compiled || !reflect.DeepEqual(p, g.runtime) || int64(len(source)) > g.limits.SourceBytes {
		return o, errors.New("cached compilation rejected")
	}
	g.compiled = true
	src, err := g.upload(ctx, []byte(source), "source")
	defer func() { g.cleanup(&err, src) }()
	if err != nil {
		return o, err
	}
	exe, err := g.reserve(int64(p.ExecutableBytes), "executable")
	if err != nil {
		return o, err
	}
	if err = g.step(0, false); err != nil {
		g.release(exe)
		return o, err
	}
	c := command(p, p.CompileArgs, p.Compile, nil)
	g.boundDiagnostics(c, true)
	c["copyIn"] = map[string]any{"main.cpp": map[string]any{"fileId": src.id}}
	c["copyOutCached"] = []string{"main?"}
	c["copyOutMax"] = p.ExecutableBytes
	r, err := g.control.result(ctx, c)
	if err != nil {
		return o, cachedError(ctx, "cached compile receipt unavailable", err)
	}
	if len(r.IDs) != 0 {
		if len(r.IDs) != 1 || r.IDs["main"] == "" {
			return o, errors.New("unexpected cached compile identities")
		}
		if err = g.adopt(exe, r.IDs["main"]); err != nil {
			return o, err
		}
	} else if r.Status != "Accepted" {
		g.release(exe)
	}
	o, err = g.observation(r, p, true)
	if err == nil {
		_, err = Classify(o, true)
	}
	if err == nil && o.Status == "Accepted" && o.Exit == 0 && exe.id == "" {
		err = errors.New("cached executable missing")
	}
	if exe.id != "" {
		g.executable = exe.id
	}
	return o, err
}

// Run uses only its compiled executable, a verified cached input and one raw
// stdout object. Both point objects are deleted before this method completes.
func (g *CachedGoJudge) Run(ctx context.Context, p experiment.ExecutionProfile, id string, input []byte) (o Observation, err error) {
	if err = g.acquire(ctx, false); err != nil {
		return o, err
	}
	defer func() { err = g.finish(ctx, err) }()
	if !g.verified || !g.compiled || id != g.executable || g.objects[id] == nil || !reflect.DeepEqual(p, g.runtime) {
		return o, errors.New("cached execution rejected")
	}
	in, err := g.upload(ctx, input, "input")
	var out *cachedObject
	defer func() { g.cleanup(&err, out, in) }()
	if err != nil {
		return o, err
	}
	out, err = g.reserve(int64(p.OutputBytes+1), "stdout")
	if err != nil {
		return o, err
	}
	if err = g.step(0, false); err != nil {
		g.release(out)
		out = nil
		return o, err
	}
	c := command(p, p.RunArgs, p.Run, nil)
	g.boundDiagnostics(c, false)
	c["files"].([]any)[0] = map[string]any{"fileId": in.id}
	c["copyIn"] = map[string]any{"main": map[string]any{"fileId": id}}
	c["copyOutCached"] = []string{"stdout"}
	c["copyOutMax"] = p.OutputBytes + 1
	r, err := g.control.result(ctx, c)
	if err != nil {
		return o, cachedError(ctx, "cached run receipt unavailable", err)
	}
	if len(r.IDs) != 1 || r.IDs["stdout"] == "" || len(r.Files) != 1 || r.Files["stderr"] == nil {
		return o, errors.New("unexpected cached output roles")
	}
	if err = g.adopt(out, r.IDs["stdout"]); err != nil {
		return o, err
	}
	raw, err := g.read(ctx, out, int64(p.OutputBytes+1))
	if err != nil {
		return o, err
	}
	if len(raw) > p.OutputBytes {
		truncated := false
		for _, e := range r.FileErrors {
			if e.Name == "stdout" && e.Type == "CollectSizeExceeded" {
				truncated = true
			}
		}
		if !truncated {
			return o, errors.New("cached output exceeds execution budget")
		}
	}
	// Validate the cache representation first, then normalize raw bytes solely
	// for the unchanged inline evidence validator; no JSON string roundtrip.
	text := string(raw)
	r.Files["stdout"] = &text
	r.IDs = nil
	o, err = g.observation(r, p, false)
	if err == nil {
		_, err = Classify(o, false)
	}
	return o, err
}

// Remove accepts only this operation's owned executable, including after error.
func (g *CachedGoJudge) Remove(ctx context.Context, id string) (err error) {
	if err = g.acquire(ctx, true); err != nil {
		return err
	}
	defer func() { err = g.finish(ctx, err) }()
	object := g.objects[id]
	if object == nil || object.role != "executable" || id != g.executable {
		return errors.New("cached artifact is not owned")
	}
	return g.remove(ctx, object)
}

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
	c17                        bool
	busy                       atomic.Bool
	verified, compiled, failed bool
	executable                 string
	compilationEntry           string // empty for the unchanged single-source protocol
	runOutputBytes             int    // zero uses the verified runtime's regular stdout limit
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

// NewReferenceGoJudgeForTier binds one reviewed runtime and file budget set.
// Unknown tiers fail before constructing a usable adapter.
func NewReferenceGoJudgeForTier(transport http.RoundTripper, verify func(context.Context, experiment.ExecutionProfile) error, files FileTransport, tier experiment.ReferenceTier) (*CachedGoJudge, error) {
	c, err := experiment.ResolveReferenceTier(tier)
	if err != nil {
		return nil, err
	}
	g := NewReferenceGoJudge(transport, verify, files)
	g.runtime, g.limits = c.Runtime, c.Files
	g.compilationEntry = c.Entry
	return g, nil
}

// NewAPlusBCacheFactory creates an independent attempt ledger for the Worker fixture.
func NewAPlusBCacheFactory(transport http.RoundTripper, verify func(context.Context, experiment.ExecutionProfile) error, files FileTransport, fixture experiment.WorkerFixture) func() *CachedGoJudge {
	tier := fixture.APlusBTier()
	if tier == experiment.ReferenceSmall {
		return func() *CachedGoJudge {
			g := NewReferenceGoJudge(transport, verify, files)
			g.c17 = true
			return g
		}
	}
	if tier != experiment.ReferenceAPlusBUbuntu {
		return nil
	}
	return func() *CachedGoJudge {
		// The tier has been admitted by the fixed fixture parser.
		g, _ := NewReferenceGoJudgeForTier(transport, verify, files, tier)
		g.c17 = true
		return g
	}
}

// NewConvolutionComparisonGoJudge retains the convolution host profile but
// reserves only the output that the fixed small-corpus parser can accept.
func NewConvolutionComparisonGoJudge(transport http.RoundTripper, verify func(context.Context, experiment.ExecutionProfile) error, files FileTransport) (*CachedGoJudge, error) {
	g, err := NewReferenceGoJudgeForTier(transport, verify, files, experiment.ReferenceConvolution)
	if err != nil {
		return nil, err
	}
	g.runOutputBytes = experiment.ConvolutionComparisonOutputBytes
	return g, nil
}

// Verify admits one fixed runtime and probes the actual trusted host/executor.
func (g *CachedGoJudge) Verify(ctx context.Context, p experiment.ExecutionProfile) (err error) {
	if err = g.acquire(ctx, false); err != nil {
		return tagFailure(err, "verify", "environment", "")
	}
	defer func() { err = g.finish(ctx, err) }()
	if g.verified || g.files == nil || !(reflect.DeepEqual(p, g.runtime) || g.c17 && reflect.DeepEqual(p, experiment.C17Profile(g.runtime))) {
		return failureError("verify", "environment", "invalid_evidence", "unsupported cached sandbox profile")
	}
	if err = g.step(0, false); err != nil {
		return tagFailure(err, "verify", "environment", "")
	}
	if err = g.step(0, false); err != nil {
		return tagFailure(err, "verify", "environment", "")
	}
	if err = g.control.Verify(ctx, p); err != nil {
		return tagFailure(cachedError(ctx, "cached sandbox verification failed", err), "verify", "environment", "")
	}
	g.runtime = p
	g.verified = true
	return nil
}

// Compile verifies uploaded source bytes, compiles once, and retains only its
// owned executable. Source deletion is independently bounded even on failure.
func (g *CachedGoJudge) Compile(ctx context.Context, p experiment.ExecutionProfile, source string) (o Observation, err error) {
	return g.compileSources(ctx, p, source, nil, false)
}

// CompileFiles admits dependencies only through the separately fixed runtime.
func (g *CachedGoJudge) CompileFiles(ctx context.Context, p experiment.ExecutionProfile, files []SourceFile) (Observation, error) {
	return g.compileSources(ctx, p, "", files, true)
}

func (g *CachedGoJudge) compileSources(ctx context.Context, p experiment.ExecutionProfile, source string, files []SourceFile, multiple bool) (o Observation, err error) {
	if err = g.acquire(ctx, false); err != nil {
		return o, tagFailure(err, "validate", "source", "")
	}
	defer func() { err = g.finish(ctx, err) }()
	if !g.verified || g.compiled || !reflect.DeepEqual(p, g.runtime) {
		return o, failureError("validate", "source", "invalid_evidence", "cached compilation rejected")
	}
	files, entry, valid := g.prepareCompilation(source, files, multiple)
	if !valid {
		return o, failureError("validate", "source", "invalid_evidence", "cached compilation rejected")
	}
	g.compiled = true
	var objects []*cachedObject
	defer func() { g.cleanup(&err, objects...) }()
	var copyIn map[string]any
	copyIn, objects, err = g.uploadCompilation(ctx, files, entry)
	if err != nil {
		return o, err
	}
	exe, err := g.reserve(int64(p.ExecutableBytes), "executable")
	if err != nil {
		return o, tagFailure(err, "control", "executable", "")
	}
	if err = g.step(0, false); err != nil {
		g.release(exe)
		return o, tagFailure(err, "control", "executable", "")
	}
	c := command(p, p.CompileArgs, p.Compile, nil)
	g.boundDiagnostics(c, true)
	c["copyIn"] = copyIn
	c["copyOutCached"] = []string{"main?"}
	c["copyOutMax"] = p.ExecutableBytes
	r, err := g.control.result(ctx, c)
	if err != nil {
		return o, tagFailure(cachedError(ctx, "cached compile receipt unavailable", err), "control", "executable", "")
	}
	if len(r.IDs) != 0 {
		if len(r.IDs) != 1 || r.IDs["main"] == "" {
			return o, failureError("validate", "executable", "ownership", "unexpected cached compile identities")
		}
		if err = g.adopt(exe, r.IDs["main"]); err != nil {
			return o, tagFailure(err, "validate", "executable", "")
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
	return o, tagFailure(err, "validate", "executable", "invalid_evidence")
}

// Run uses only its compiled executable, a verified cached input and one raw
// stdout object. Both point objects are deleted before this method completes.
func (g *CachedGoJudge) Run(ctx context.Context, p experiment.ExecutionProfile, id string, input []byte) (Observation, error) {
	return g.run(ctx, p, id, input, false)
}

// RunCheckerTrial requires complete byte evidence from the reference cache.
// It deliberately does not widen the output rules of the other run methods.
func (g *CachedGoJudge) RunCheckerTrial(ctx context.Context, p experiment.ExecutionProfile, id string, input []byte) (Observation, error) {
	return g.run(ctx, p, id, input, true)
}

func (g *CachedGoJudge) run(ctx context.Context, p experiment.ExecutionProfile, id string, input []byte, checkerTrial bool) (o Observation, err error) {
	if err = g.acquire(ctx, false); err != nil {
		return o, tagFailure(err, "validate", "executable", "")
	}
	defer func() { err = g.finish(ctx, err) }()
	if checkerTrial && !g.reference {
		return o, failureError("validate", "stdout", "invalid_evidence", "checker trial requires the reference cache")
	}
	if !g.verified || !g.compiled || id != g.executable || g.objects[id] == nil || !reflect.DeepEqual(p, g.runtime) {
		return o, failureError("validate", "executable", "ownership", "cached execution rejected")
	}
	if g.runOutputBytes > 0 {
		p.OutputBytes = min(p.OutputBytes, g.runOutputBytes)
	}
	in, err := g.upload(ctx, input, "input")
	var out *cachedObject
	defer func() { g.cleanup(&err, out, in) }()
	if err != nil {
		return o, err
	}
	out, err = g.reserve(int64(p.OutputBytes+1), "stdout")
	if err != nil {
		return o, tagFailure(err, "control", "stdout", "")
	}
	if err = g.step(0, false); err != nil {
		g.release(out)
		out = nil
		return o, tagFailure(err, "control", "stdout", "")
	}
	c := command(p, p.RunArgs, p.Run, nil)
	g.boundDiagnostics(c, false)
	c["files"].([]any)[0] = map[string]any{"fileId": in.id}
	c["copyIn"] = map[string]any{"main": map[string]any{"fileId": id}}
	c["copyOutCached"] = []string{"stdout"}
	c["copyOutMax"] = p.OutputBytes + 1
	r, err := g.control.result(ctx, c)
	if err != nil {
		return o, tagFailure(cachedError(ctx, "cached run receipt unavailable", err), "control", "stdout", "")
	}
	if len(r.IDs) != 1 || r.IDs["stdout"] == "" || len(r.Files) != 1 || r.Files["stderr"] == nil {
		return o, failureError("validate", "stdout", "ownership", "unexpected cached output roles")
	}
	if err = g.adopt(out, r.IDs["stdout"]); err != nil {
		return o, tagFailure(err, "validate", "stdout", "")
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
			return o, failureError("validate", "stdout", "invalid_evidence", "cached output exceeds execution budget")
		}
	}
	// Validate the cache representation first, then normalize raw bytes solely
	// for the unchanged inline evidence validator; no JSON string roundtrip.
	text := string(raw)
	r.Files["stdout"] = &text
	r.IDs = nil
	if checkerTrial && !checkerTrialOutputEvidence(r, p) {
		return o, failureError("validate", "stdout", "invalid_evidence", "contradictory checker trial output evidence")
	}
	o, err = g.observation(r, p, false)
	if err == nil {
		_, err = Classify(o, false)
	}
	return o, tagFailure(err, "validate", "stdout", "invalid_evidence")
}

// Remove accepts only this operation's owned executable, including after error.
func (g *CachedGoJudge) Remove(ctx context.Context, id string) (err error) {
	if err = g.acquire(ctx, true); err != nil {
		return tagFailure(err, "delete", "executable", "")
	}
	defer func() { err = g.finish(ctx, err) }()
	object := g.objects[id]
	if object == nil || object.role != "executable" || id != g.executable {
		return failureError("delete", "executable", "ownership", "cached artifact is not owned")
	}
	return g.remove(ctx, object)
}

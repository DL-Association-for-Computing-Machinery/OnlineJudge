package execution

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
)

// checkSPJ uses a separate cache ledger from the contestant. Each call
// provisions a fresh process and its exact four files. Nothing except the
// verdict leaves this operation.
func (g *CachedGoJudge) checkSPJ(ctx context.Context, checker, input, output, answer []byte) (ok bool, err error) {
	if err = g.acquire(ctx, false); err != nil {
		return false, err
	}
	defer func() {
		err = g.finish(ctx, err)
		if err != nil {
			ok = false
		}
	}()
	if !g.verified || !g.reference || g.runtime.ImageID != experiment.PythonImageID || g.compiled ||
		len(checker) == 0 || len(checker) > experiment.SPJArtifactBytes ||
		len(input) > experiment.SPJObjectBytes || len(output) > experiment.SPJObjectBytes || len(answer) > experiment.SPJObjectBytes {
		return false, judging.ErrCheckerResult
	}
	limits := experiment.SPJLimits()
	args := experiment.SPJArgs()
	c := command(g.runtime, args[:], limits, nil)
	c["files"] = []any{map[string]any{"content": ""}, map[string]any{"name": "stdout", "max": experiment.ReferenceDiagnosticBytes},
		map[string]any{"name": "stderr", "max": experiment.ReferenceDiagnosticBytes}}
	copyIn := make(map[string]any, 4)
	var objects []*cachedObject
	defer func() { g.cleanup(&err, objects...) }()
	for _, item := range []struct {
		name string
		data []byte
	}{{"checker", checker}, {"input", input}, {"output", output}, {"answer", answer}} {
		obj, uploadErr := g.upload(ctx, item.data, item.name)
		if obj != nil {
			objects = append(objects, obj)
		}
		if uploadErr != nil {
			return false, uploadErr
		}
		copyIn[item.name] = map[string]any{"fileId": obj.id}
	}
	c["copyIn"] = copyIn
	if err := g.step(0, false); err != nil {
		return false, err
	}
	result, err := g.control.result(ctx, c)
	if err != nil {
		return false, err
	}
	outcome, err := verifyCheckerResult(result, judging.CheckerTestlib0945, limits, experiment.ReferenceDiagnosticBytes)
	return outcome == judging.CheckerAccept, err
}

func (r *Runner) spjChecker(ctx context.Context, p manifest) (func([]byte, []byte, []byte) (bool, error), error) {
	if r.newCached == nil {
		return nil, judging.ErrCheckerResult
	}
	artifact := r.fixture.CheckerArtifact()
	if len(artifact) == 0 {
		return nil, judging.ErrCheckerResult
	}
	cache := r.newCached()
	cache.limits = experiment.SPJFiles()
	preflight, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := cache.Verify(preflight, p.Environment)
	cancel()
	if err != nil {
		return nil, err
	}
	return func(input, output, answer []byte) (bool, error) {
		call, cancel := context.WithTimeout(ctx, time.Duration(experiment.SPJLimits().Wall)+5*time.Second)
		defer cancel()
		ok, err := cache.checkSPJ(call, artifact, input, output, answer)
		return ok, errors.Join(err, ctx.Err())
	}, nil
}

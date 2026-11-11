package problemvalidation

import (
	"context"
	"errors"
	"io"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

// These pins mirror the reviewed source lock. The Worker has no author bundle
// or source-lock file; plan metadata from transport cannot choose its programs.
const convolutionSourceRevision = "73876f1cbcf082d2b22722924b924d3d65a2f00e"

func convolutionPins() (oracle, candidate ConvolutionProgram) {
	return ConvolutionProgram{Path: convolutionOraclePath, Size: 903, SHA256: "34539b27c9630df09396f94e8211fd83fd87d85b05e313361d58ada97942a3c9"},
		ConvolutionProgram{Path: convolutionCandidatePath, Size: 3284, SHA256: "874160d0617fd4c7da455c452ee7ac6bdc34ddf0614a818fcc4b8ef411990f62"}
}

// MaterialObjects returns the only allowed objects after checking the current
// trial contract, independently of the control plane's historical read check.
func (p ConvolutionPlan) MaterialObjects(formalSHA256 string) ([2]materialstore.Object, error) {
	oracle, candidate := convolutionPins()
	profile, _, err := ResolveSource(ConvolutionSourceID)
	if err != nil || !p.ValidIdentity() || p.FormalSHA256 != formalSHA256 ||
		p.SourceLock != SourceLockSHA256 || p.SourceRevision != convolutionSourceRevision ||
		p.Oracle != oracle || p.Candidate != candidate || p.ProfileSHA256 != profile.Digest() ||
		p.CorpusSHA256 != convolutionCorpusDigest(smallConvolutionCorpus()) {
		return [2]materialstore.Object{}, errConvolutionSource
	}
	return [2]materialstore.Object{{SHA256: oracle.SHA256, Size: oracle.Size}, {SHA256: candidate.SHA256, Size: candidate.Size}}, nil
}

// FreezePrograms verifies and closes both bounded objects before returning
// immutable compilation bytes. Neither transport metadata nor one valid object
// can produce a partial pair. The caller owns reservation supervision.
func (p ConvolutionPlan) FreezePrograms(ctx context.Context, formalSHA256 string, open func(context.Context, materialstore.Object) (io.ReadCloser, error)) (ConvolutionSources, error) {
	objects, err := p.MaterialObjects(formalSHA256)
	if err != nil || ctx == nil || open == nil {
		return ConvolutionSources{}, errConvolutionSource
	}
	var sources [2]string
	for i, o := range objects {
		if err := ctx.Err(); err != nil {
			return ConvolutionSources{}, err
		}
		body, err := open(ctx, o)
		if err != nil || body == nil {
			if body != nil {
				_ = body.Close()
			}
			return ConvolutionSources{}, errConvolutionSource
		}
		data, err := io.ReadAll(io.LimitReader(body, o.Size+1))
		if err := errors.Join(err, body.Close(), ctx.Err()); err != nil {
			return ConvolutionSources{}, err
		}
		if int64(len(data)) != o.Size || convolutionSHA256(data) != o.SHA256 {
			return ConvolutionSources{}, errConvolutionSource
		}
		sources[i] = string(data)
	}
	return ConvolutionSources{revision: p.SourceRevision, draft: p.Draft,
		oracle:    convolutionProgram{identity: p.Oracle, source: sources[0]},
		candidate: convolutionProgram{identity: p.Candidate, source: sources[1]}}, nil
}

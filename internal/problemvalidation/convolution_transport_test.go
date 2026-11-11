package problemvalidation

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

func TestConvolutionTransportChecksPinsBeforeAnyRead(t *testing.T) {
	formal := strings.Repeat("a", 64)
	oracle, candidate := convolutionPins()
	fixture := ConvolutionSources{
		revision: convolutionSourceRevision, oracle: convolutionProgram{identity: oracle, source: "test oracle"},
		candidate: convolutionProgram{identity: candidate, source: "test candidate"},
		draft:     ConvolutionDraftBinding{ID: strings.Repeat("1", 32), Version: formal, ImportRevision: formal},
	}
	plan, err := fixture.DraftPlan(formal)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*ConvolutionPlan)
	}{
		{"role swap", func(p *ConvolutionPlan) { p.Oracle, p.Candidate = p.Candidate, p.Oracle }},
		{"different source bytes", func(p *ConvolutionPlan) { p.Oracle.SHA256 = formal }},
		{"different source revision", func(p *ConvolutionPlan) { p.SourceRevision = strings.Repeat("1", 40) }},
		{"different profile", func(p *ConvolutionPlan) { p.ProfileSHA256 = formal }},
		{"different corpus", func(p *ConvolutionPlan) { p.CorpusSHA256 = formal }},
		{"different formal plan", func(p *ConvolutionPlan) { p.FormalSHA256 = strings.Repeat("b", 64) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := plan
			tc.edit(&changed)
			result, err := changed.FreezePrograms(t.Context(), formal, func(context.Context, materialstore.Object) (io.ReadCloser, error) {
				t.Fatal("unreviewed plan reached object I/O")
				return nil, nil
			})
			if result != (ConvolutionSources{}) || err == nil {
				t.Fatal("unreviewed plan returned a snapshot")
			}
		})
	}
	count := 0
	result, err := plan.FreezePrograms(t.Context(), formal, func(_ context.Context, object materialstore.Object) (io.ReadCloser, error) {
		count++
		return io.NopCloser(strings.NewReader(strings.Repeat("x", int(object.Size)))), nil
	})
	if count != 1 || result != (ConvolutionSources{}) || err == nil {
		t.Fatal("same-size corruption advanced to the second program or returned partial data")
	}
}

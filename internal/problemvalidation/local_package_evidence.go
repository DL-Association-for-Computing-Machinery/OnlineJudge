package problemvalidation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

// PackageQualityEvidence is the bounded reviewer-facing projection of two
// operator-pinned runs, bound to the same imported source and formal plan.
type PackageQualityEvidence struct {
	Validator InputValidatorEvidence
	Oracle    IndependentCorrectnessEvidence
	Coverage  TestCoverageEvidence
	Rendering StatementRenderingEvidence
}

type packageRendering struct {
	Schema         string `json:"schema"`
	ImportRevision string `json:"import_revision"`
	Desktop        bool   `json:"desktop"`
	Narrow         bool   `json:"narrow"`
	Examples       int    `json:"examples"`
	Formulas       int    `json:"formulas"`
	FormatBlocks   int    `json:"format_blocks"`
}

func verifyPackageQuality(ctx context.Context, p *problem.Prepared, plan problem.ReferencePlan, in LocalTextAdmissionInput) (PackageQualityEvidence, error) {
	var out PackageQualityEvidence
	q, err := readPackageQuality(ctx, p)
	if err != nil {
		return out, err
	}
	complete := func() bool {
		_, err := os.Lstat(filepath.Join(in.QualityDirectory, "INCOMPLETE"))
		return errors.Is(err, os.ErrNotExist)
	}
	if !complete() {
		return out, ErrCheckerTrialEvidence
	}
	raw, err := checkedfile.ReadSecret(filepath.Join(in.QualityDirectory, "report.json"), 64<<10)
	var r PackageQualityReport
	if err != nil || handoffDigest(raw) != in.QualitySHA256 || json.Unmarshal(raw, &r) != nil {
		return out, ErrCheckerTrialEvidence
	}
	encoded, err := json.MarshalIndent(r, "", "  ")
	expected := q.report(plan)
	if err != nil || !bytes.Equal(raw, append(encoded, '\n')) || !r.StopConfirmed || r.Schema != expected.Schema ||
		r.ImportRevision != expected.ImportRevision || r.Plan != expected.Plan || r.Profile != expected.Profile ||
		r.Config != expected.Config || r.Inputs != expected.Inputs || r.SamplePoints != expected.SamplePoints ||
		r.Controls != expected.Controls || r.OracleCases != expected.OracleCases || r.Criteria != expected.Criteria || len(r.Wrong) != 4 {
		return out, ErrCheckerTrialEvidence
	}
	for i, rejected := range r.Wrong {
		if len(rejected) < 1 || len(rejected) > r.Inputs-r.SamplePoints {
			return PackageQualityEvidence{}, ErrCheckerTrialEvidence
		}
		for j, point := range rejected {
			if !q.nonSample(point) || j > 0 && point <= rejected[j-1] {
				return PackageQualityEvidence{}, ErrCheckerTrialEvidence
			}
		}
		for _, prev := range r.Wrong[:i] {
			if slices.Equal(prev, rejected) {
				return PackageQualityEvidence{}, ErrCheckerTrialEvidence
			}
		}
		out.Coverage.WrongSolutions[i] = CoverageWrongSolution{Candidate: q.config.Wrong[i], RejectedPoints: len(rejected)}
	}
	raw, err = checkedfile.ReadSecret(in.RenderingFile, 16<<10)
	var display packageRendering
	if err != nil || handoffDigest(raw) != in.RenderingSHA256 || json.Unmarshal(raw, &display) != nil {
		return PackageQualityEvidence{}, ErrCheckerTrialEvidence
	}
	encoded, err = json.Marshal(display)
	if err != nil || !bytes.Equal(raw, append(encoded, '\n')) || display.Schema != "oj.local-text-package-rendering.v1" ||
		display.ImportRevision != plan.ImportRevision || !display.Desktop || !display.Narrow || display.Examples != r.SamplePoints ||
		display.Formulas < 0 || display.Formulas > 128 || display.FormatBlocks < 0 || display.FormatBlocks > 128 || !complete() {
		return PackageQualityEvidence{}, ErrCheckerTrialEvidence
	}
	out.Validator = InputValidatorEvidence{ReportSHA256: in.QualitySHA256, Inputs: r.Inputs, Controls: r.Controls}
	out.Oracle = IndependentCorrectnessEvidence{ReportSHA256: in.QualitySHA256, Suite: PackageQualitySchema, Cases: r.OracleCases}
	out.Coverage.ReportSHA256, out.Coverage.Points, out.Coverage.SamplePoints, out.Coverage.NonSampleCriteria = in.QualitySHA256, r.Inputs, r.SamplePoints, r.Criteria
	out.Rendering = StatementRenderingEvidence{ReportSHA256: in.RenderingSHA256, Examples: display.Examples, Formulas: display.Formulas, FormatBlocks: display.FormatBlocks}
	return out, nil
}

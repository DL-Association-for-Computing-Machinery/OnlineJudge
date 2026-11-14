package problemvalidation

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

type spjReport struct {
	Schema   string     `json:"schema"`
	Source   string     `json:"source_sha256"`
	Artifact string     `json:"artifact_sha256"`
	Image    string     `json:"image"`
	Header   string     `json:"adapted_header_sha256"`
	Plan     string     `json:"plan_sha256"`
	Runtime  string     `json:"runtime_sha256"`
	Points   []spjPoint `json:"points"`
}

type spjPoint struct {
	Input     string `json:"input_sha256"`
	Answer    string `json:"answer_sha256"`
	Canonical int    `json:"canonical_exit"`
	Alternate int    `json:"alternate_exit"`
	Invalid   int    `json:"invalid_exit"`
}

func bindSPJReport(ctx context.Context, prepared *problem.Prepared, plan problem.ReferencePlan, in LocalTextAdmissionInput, manifest *materials.TextTransportManifest) error {
	raw, err := checkedfile.ReadSecret(in.SPJReport, 16<<10)
	if err != nil || handoffDigest(raw) != in.SPJReportSHA256 {
		return ErrCheckerTrialEvidence
	}
	source, err := prepared.ReadLocalTextQualityFile(ctx, "quality/checker.cpp", 64<<10)
	if err != nil {
		return ErrCheckerTrialEvidence
	}
	report, err := verifySPJReport(raw, plan, handoffDigest(source))
	if err != nil {
		return err
	}
	manifest.Checker, manifest.CheckerArtifact, manifest.TokenBytes = materials.LocalSPJChecker, report.Artifact, 0
	manifest.RuntimeSHA256 = report.Runtime
	manifest.JudgeSHA256 = digest([4]string{manifest.JudgeSHA256, manifest.Checker, manifest.RuntimeSHA256, in.SPJReportSHA256})
	manifest.Version = "local-text-" + manifest.JudgeSHA256
	return ctx.Err()
}

func verifySPJReport(raw []byte, plan problem.ReferencePlan, source string) (spjReport, error) {
	var report spjReport
	if json.Unmarshal(raw, &report) != nil {
		return spjReport{}, ErrCheckerTrialEvidence
	}
	canonical, err := json.Marshal(report)
	if err != nil || !bytes.Equal(raw, canonical) || report.Schema != materials.LocalSPJChecker ||
		report.Image != experiment.PythonImageID || report.Header != experiment.SPJHeaderSHA256 ||
		report.Plan != plan.Digest() || report.Runtime != experiment.LocalSPJRuntimeSHA256() ||
		report.Source != source || !problem.ValidImportRevision(report.Artifact) ||
		len(report.Points) == 0 || len(report.Points) != len(plan.Cases) || len(report.Points) > materials.LocalSPJPoints {
		return spjReport{}, ErrCheckerTrialEvidence
	}
	for i, point := range plan.Cases {
		r := report.Points[i]
		if r.Input != point.Input.SHA256 || r.Answer != point.Answer.SHA256 ||
			point.Input.Size > experiment.SPJObjectBytes || point.Answer.Size > experiment.SPJObjectBytes ||
			r.Canonical != 50 || r.Alternate != 50 || r.Invalid != 51 {
			return spjReport{}, ErrCheckerTrialEvidence
		}
	}
	return report, nil
}

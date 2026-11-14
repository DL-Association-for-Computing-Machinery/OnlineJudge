package problemvalidation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

var localTextLimits = judging.TextLimits{Bytes: materials.APlusBOutputBytes, TokenBytes: 4096}

const localTextTrialWall = 3 * time.Minute

// LocalTextAdmissionInput refers to the complete approved checker preflight
// and independently pinned executable manifest. Source and runtime are closed.
type LocalTextAdmissionInput struct {
	SourceID            string `json:"source_id"`
	ReportDirectory     string `json:"report_directory"`
	ReportSHA256        string `json:"report_sha256"`
	ReviewProfileSHA256 string `json:"review_profile_sha256"`
	ManifestSHA256      string `json:"manifest_sha256"`
	PackageReview       bool   `json:"package_review,omitempty"`
	QualityDirectory    string `json:"quality_directory,omitempty"`
	QualitySHA256       string `json:"quality_sha256,omitempty"`
	RenderingFile       string `json:"rendering_file,omitempty"`
	RenderingSHA256     string `json:"rendering_sha256,omitempty"`
	SPJReport           string `json:"spj_report,omitempty"`
	SPJReportSHA256     string `json:"spj_report_sha256,omitempty"`
}

// Valid bounds trusted local configuration before filesystem access.
func (in LocalTextAdmissionInput) Valid() bool {
	return in.validEvidence() && problem.ValidImportRevision(in.ManifestSHA256)
}

func (in LocalTextAdmissionInput) validEvidence() bool {
	if in.SourceID == "" || !absoluteLocalPath(in.ReportDirectory) || !problem.ValidImportRevision(in.ReportSHA256) ||
		!problem.ValidImportRevision(in.ReviewProfileSHA256) {
		return false
	}
	if (in.SPJReport != "" || in.SPJReportSHA256 != "") &&
		(!in.PackageReview || !absoluteLocalPath(in.SPJReport) || !problem.ValidImportRevision(in.SPJReportSHA256)) {
		return false
	}
	if in.PackageReview {
		return in.ReviewProfileSHA256 == LocalTextProfile().Digest() && absoluteLocalPath(in.QualityDirectory) &&
			problem.ValidImportRevision(in.QualitySHA256) && absoluteLocalPath(in.RenderingFile) && problem.ValidImportRevision(in.RenderingSHA256)
	}
	return in.QualityDirectory == "" && in.QualitySHA256 == "" && in.RenderingFile == "" && in.RenderingSHA256 == ""
}

// LocalTextProfile is the fixed GNU++17 Ubuntu maintenance profile.
func LocalTextProfile() Profile {
	c, _ := experiment.ResolveReferenceTier(experiment.ReferenceAPlusBUbuntu)
	return Profile{ID: "p5-local-text-reference-v1", Runtime: c.Runtime, Files: c.Files, Materials: problem.LocalTextReferenceLimits(),
		Checker: judging.LibraryCheckerTextVersion, Text: localTextLimits, Wall: localTextTrialWall, Diagnostics: experiment.ReferenceDiagnosticBytes}
}

// PrepareLocalTextTrial derives the formal plan and fixed checker configuration
// from the source-locked bundle. It has no per-problem profile overrides.
func PrepareLocalTextTrial(ctx, lifetime context.Context, prepared *problem.Prepared, wrongSource []byte, packageReview bool) (_ *CheckerTrial, err error) {
	if len(wrongSource) > submission.MaxSourceBytes || !utf8.Valid(wrongSource) || bytes.IndexByte(wrongSource, 0) >= 0 {
		return nil, ErrCheckerCandidateBinding
	}
	freeze := prepared.PrepareLocalTextReference
	if packageReview {
		freeze = prepared.PrepareLocalTextReviewReference
	}
	snapshot, err := freeze(ctx, lifetime)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, snapshot.Close())
		}
	}()
	plan := snapshot.Plan()
	var total int64
	for _, point := range plan.Cases {
		total += point.Input.Size + point.Answer.Size
	}
	if len(plan.Cases) == 0 || total > materials.LocalTextTotalBytes {
		return nil, ErrCheckerCandidateBinding
	}
	planSHA := plan.Digest()
	if err := checkCandidateAnswers(ctx, plan, snapshot, planSHA, localTextLimits); err != nil {
		return nil, err
	}
	profile := LocalTextProfile()
	candidate := checkerCandidateFor(plan, planSHA, profile.Checker, localTextLimits)
	trial := &CheckerTrial{plan: plan, materials: snapshot, profile: profile, tier: experiment.ReferenceAPlusBUbuntu, candidateSHA: candidate.Digest()}
	if len(wrongSource) != 0 {
		trial.submissionSource, trial.submissionSHA = bytes.Clone(wrongSource), handoffDigest(wrongSource)
	}
	return trial, nil
}

// VerifyLocalText derives all ordered materials from the reviewed bundle and
// checks an actual checker run on the fixed local runtime.
// Export may stage verified objects privately; the caller commits only on success.
func VerifyLocalText(ctx context.Context, prepared *problem.Prepared, in LocalTextAdmissionInput, export func(string, []byte) error) (_ LocalExecutionAdmission, err error) {
	var zero LocalExecutionAdmission
	if ctx == nil || prepared == nil || !in.validEvidence() {
		return zero, ErrCheckerCandidateBinding
	}
	id, err := prepared.ImportIdentity()
	if err != nil || id.SourceID != in.SourceID || in.PackageReview && id.SourceID == ConvolutionSourceID {
		return zero, ErrCheckerCandidateBinding
	}
	spj, err := prepared.DeclaresLocalSPJ()
	if err != nil || spj != (in.SPJReport != "") {
		return zero, ErrCheckerCandidateBinding
	}
	checkComplete := func() bool {
		_, err := os.Lstat(filepath.Join(in.ReportDirectory, "INCOMPLETE"))
		return errors.Is(err, os.ErrNotExist)
	}
	if !checkComplete() {
		return zero, ErrCheckerTrialEvidence
	}
	raw, err := checkedfile.ReadSecret(filepath.Join(in.ReportDirectory, "report.json"), MaxCheckerTrialReportBytes)
	if err != nil {
		return zero, err
	}
	trial, err := PrepareLocalTextTrial(ctx, ctx, prepared, nil, in.PackageReview)
	if err != nil {
		return zero, err
	}
	defer func() { err = errors.Join(err, trial.Close()) }()
	proof, err := trial.VerifyPassingReport(raw, in.ReportSHA256, "oj-aplusb-ubuntu-sandbox")
	if err != nil {
		return zero, ErrCheckerTrialEvidence
	}
	runtime := experiment.APlusBRuntimeSHA256ForTier(experiment.ReferenceAPlusBUbuntu)
	binding := digest(struct{ Plan, Candidate, Runtime string }{proof.FormalPlanSHA, proof.CandidateSHA256, runtime})
	m := materials.TextTransportManifest{Origin: materials.LocalTextOrigin, Version: "local-text-" + binding,
		JudgeSHA256: binding, RuntimeSHA256: runtime, Checker: judging.LibraryCheckerTextVersion, Language: "gnu++17", TokenBytes: localTextLimits.TokenBytes}
	if in.SPJReport != "" {
		if err := bindSPJReport(ctx, prepared, trial.plan, in, &m); err != nil {
			return zero, err
		}
	}
	for i, point := range trial.plan.Cases {
		m.Tests = append(m.Tests, materials.TextTransportPoint{ID: point.ID, Input: point.Input.SHA256, Answer: point.Answer.SHA256})
		if export != nil {
			input, err := trial.materials.ReadInput(ctx, i)
			if err != nil {
				return zero, err
			}
			answer, n, err := trial.materials.AnswerAt(ctx, i)
			if err != nil || n != point.Answer.Size {
				return zero, ErrCheckerCandidateBinding
			}
			data, err := io.ReadAll(io.NewSectionReader(answer, 0, n))
			if err != nil || int64(len(input)) != point.Input.Size || int64(len(data)) != n ||
				handoffDigest(input) != point.Input.SHA256 || handoffDigest(data) != point.Answer.SHA256 {
				return zero, ErrCheckerCandidateBinding
			}
			if err := errors.Join(export(point.Input.SHA256, input), export(point.Answer.SHA256, data)); err != nil {
				return zero, err
			}
		}
	}
	wire := m.Canonical()
	pin := handoffDigest(wire)
	if _, err := materials.ParseLocalText(wire, pin); err != nil || !checkComplete() || ctx.Err() != nil {
		return zero, ErrCheckerTrialEvidence
	}
	verified := LocalExecutionAdmission{digest: digest(proof), version: m.Version, transport: pin, raw: string(wire),
		textImport: id, textPlan: proof.FormalPlanSHA, textProfile: in.ReviewProfileSHA256, packageReview: in.PackageReview}
	if in.PackageReview {
		verified.quality, err = verifyPackageQuality(ctx, prepared, trial.plan, in)
		if err != nil {
			return zero, err
		}
		verified.digest = digest([3]string{verified.digest, in.QualitySHA256, in.RenderingSHA256})
	}
	if in.SPJReport != "" {
		verified.digest = digest([2]string{verified.digest, in.SPJReportSHA256})
	}
	return verified, nil
}

// ReadLocalTextAdmission accepts only the explicit, rederived transport pin.
func ReadLocalTextAdmission(ctx context.Context, in LocalTextAdmissionInput, prepared *problem.Prepared) (LocalExecutionAdmission, error) {
	var zero LocalExecutionAdmission
	if !in.Valid() {
		return zero, ErrCheckerCandidateBinding
	}
	a, err := VerifyLocalText(ctx, prepared, in, nil)
	if err != nil || a.textImport.SourceID != in.SourceID || a.transport != in.ManifestSHA256 || !a.ValidateManifest() {
		return zero, errors.Join(ErrCheckerCandidateBinding, err)
	}
	return a, nil
}

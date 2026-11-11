package problemvalidation

import (
	"encoding/hex"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

// ConvolutionPlan records the identity of a planned small differential run.
// It contains neither compilation bytes nor an execution result.
type ConvolutionPlan struct {
	Protocol       string                  `json:"protocol"`
	SourceID       string                  `json:"source_id"`
	SourceRevision string                  `json:"source_revision"`
	SourceLock     string                  `json:"source_lock_sha256"`
	Draft          ConvolutionDraftBinding `json:"draft"`
	Oracle         ConvolutionProgram      `json:"oracle"`
	Candidate      ConvolutionProgram      `json:"candidate"`
	Corpus         string                  `json:"corpus"`
	Seed           uint64                  `json:"seed"`
	CorpusSHA256   string                  `json:"corpus_sha256"`
	Cases          int                     `json:"cases"`
	ProfileSHA256  string                  `json:"profile_sha256"`
	FormalSHA256   string                  `json:"formal_plan_sha256"`
}

// ConvolutionPlanProtocol fixes the encoding and trial contract of this plan.
const ConvolutionPlanProtocol = "oj.convolution-differential-plan.v1"

// ValidIdentity checks the v1 historical record's shape. It does not admit the
// plan for execution; that requires current pins, corpus, profile and authority.
func (p ConvolutionPlan) ValidIdentity() bool {
	program := func(got ConvolutionProgram, path string) bool {
		return got.Path == path && got.Size > 0 && got.Size <= 64<<10 && problem.ValidImportRevision(got.SHA256)
	}
	_, err := hex.DecodeString(p.SourceRevision)
	return p.Protocol == ConvolutionPlanProtocol && p.SourceID == ConvolutionSourceID &&
		len(p.SourceRevision) == 40 && err == nil && problem.ValidImportRevision(p.SourceLock) &&
		identity.ValidID(p.Draft.ID) && problem.ValidImportRevision(p.Draft.Version) && problem.ValidImportRevision(p.Draft.ImportRevision) &&
		program(p.Oracle, convolutionOraclePath) && program(p.Candidate, convolutionCandidatePath) && p.Oracle.SHA256 != p.Candidate.SHA256 &&
		p.Corpus == convolutionCorpusID && p.Seed == convolutionCorpusSeed && p.Cases == convolutionCases &&
		problem.ValidImportRevision(p.CorpusSHA256) && problem.ValidImportRevision(p.ProfileSHA256) && problem.ValidImportRevision(p.FormalSHA256)
}

// DraftPlan is available only after both reviewed solvers have been frozen
// from a bound draft. Maintenance runs from a local source root have no plan.
func (s ConvolutionSources) DraftPlan(formalSHA256 string) (ConvolutionPlan, error) {
	profile, _, err := ResolveSource(ConvolutionSourceID)
	if err != nil || s.draft.ID == "" || s.oracle.source == "" || s.candidate.source == "" || !problem.ValidImportRevision(formalSHA256) {
		return ConvolutionPlan{}, errConvolutionSource
	}
	return ConvolutionPlan{
		Protocol: ConvolutionPlanProtocol, SourceID: ConvolutionSourceID, SourceRevision: s.revision, SourceLock: SourceLockSHA256,
		Draft: s.draft, Oracle: s.oracle.identity, Candidate: s.candidate.identity,
		Corpus: convolutionCorpusID, Seed: convolutionCorpusSeed, Cases: convolutionCases,
		CorpusSHA256: convolutionCorpusDigest(smallConvolutionCorpus()), ProfileSHA256: profile.Digest(), FormalSHA256: formalSHA256,
	}, nil
}

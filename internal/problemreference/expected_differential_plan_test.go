package problemreference

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestDifferentialPlanReadbackChecksHistoricalIdentityAndCanonicalBytes(t *testing.T) {
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	// Historical reading uses the saved lock and profile, not current admission.
	i := problem.ImportIdentity{SourceID: problemvalidation.ConvolutionSourceID, Version: a, ImportRevision: b, SourceLockSHA256: a}
	r := Request{DraftID: strings.Repeat("1", 32), Specification: Specification{Identity: i, ProfileSHA256: b}}
	formal := problem.ReferencePlan{SourceID: i.SourceID, Version: i.Version, ImportRevision: i.ImportRevision,
		SourceLockSHA256: i.SourceLockSHA256, Cases: []problem.ReferenceCase{{ID: "small"}}}
	formalBytes, err := json.Marshal(formal)
	if err != nil {
		t.Fatal(err)
	}
	comparison := problemvalidation.ConvolutionPlan{
		Protocol: problemvalidation.ConvolutionPlanProtocol, SourceID: i.SourceID, SourceRevision: strings.Repeat("1", 40), SourceLock: i.SourceLockSHA256,
		Draft:     problemvalidation.ConvolutionDraftBinding{ID: r.DraftID, Version: i.Version, ImportRevision: i.ImportRevision},
		Oracle:    problemvalidation.ConvolutionProgram{Path: "convolution/convolution_mod/sol/naive.cpp", Size: 1, SHA256: a},
		Candidate: problemvalidation.ConvolutionProgram{Path: "convolution/convolution_mod/sol/correct.cpp", Size: 2, SHA256: b},
		Corpus:    "convolution-mod-small-v1", Seed: 0x4f4a2d50342d3031, Cases: 24, CorpusSHA256: a,
		ProfileSHA256: r.Specification.ProfileSHA256, FormalSHA256: formal.Digest(),
	}
	for _, tc := range []struct {
		name string
		edit func(*problemvalidation.ConvolutionPlan)
	}{
		{name: "historical identity"},
		{name: "different draft", edit: func(p *problemvalidation.ConvolutionPlan) { p.Draft.ID = strings.Repeat("2", 32) }},
		{name: "different import", edit: func(p *problemvalidation.ConvolutionPlan) { p.Draft.ImportRevision = a }},
		{name: "different lock", edit: func(p *problemvalidation.ConvolutionPlan) { p.SourceLock = b }},
		{name: "different profile", edit: func(p *problemvalidation.ConvolutionPlan) { p.ProfileSHA256 = a }},
		{name: "different formal plan", edit: func(p *problemvalidation.ConvolutionPlan) { p.FormalSHA256 = b }},
		{name: "swapped roles", edit: func(p *problemvalidation.ConvolutionPlan) { p.Oracle, p.Candidate = p.Candidate, p.Oracle }},
		{name: "noncanonical encoding"},
		{name: "different digest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := comparison
			if tc.edit != nil {
				tc.edit(&candidate)
			}
			raw, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "noncanonical encoding" {
				raw = append(raw, '\n')
			}
			d := &DifferentialExpectedPlan{Encoding: raw, SHA256: profileDigest(raw)}
			if tc.name == "different digest" {
				d.SHA256 = b
			}
			plan := ExpectedPlan{Encoding: formalBytes, SHA256: formal.Digest(), Differential: d}
			if plan.ValidForRequest(r) != (tc.name == "historical identity") {
				t.Fatal("unexpected differential plan readback")
			}
		})
	}
}

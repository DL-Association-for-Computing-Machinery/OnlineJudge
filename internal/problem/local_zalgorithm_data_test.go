package problem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

func TestLocalZAlgorithmRejectsChangedPlanBeforeMaterialReads(t *testing.T) {
	cases := make([]ReferenceCase, zAlgorithmUpstream, zAlgorithmUpstream+len(zAlgorithmLocalCases))
	for i := range cases {
		cases[i] = ReferenceCase{ID: fmt.Sprintf("case-%04d", i+1), Input: zAlgorithmLocalCases[0].Input, Answer: zAlgorithmLocalCases[0].Answer}
	}
	a, err := referenceDiskAdmission(zAlgorithmSource)
	if err != nil {
		t.Fatal(err)
	}
	var deps []ReferenceDependency
	for _, dep := range a.dependencies {
		deps = append(deps, ReferenceDependency{Path: dep.target, Object: zAlgorithmLocalSource})
	}
	plan := ReferencePlan{
		SourceID: zAlgorithmSource, SourceLockSHA256: zAlgorithmSourceLock, Version: zAlgorithmLocalVersion,
		ImportRevision: zAlgorithmLocalVersion, SourcePath: a.entry, Source: zAlgorithmLocalSource,
		Definition: zAlgorithmDefinition, Hashes: zAlgorithmHashes, Dependencies: deps, Cases: append(cases, zAlgorithmLocalCases[:]...),
	}
	if err := plan.ValidateMaterials(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*ReferencePlan)
	}{
		{"omitted point", func(p *ReferencePlan) { p.Cases = p.Cases[:30] }},
		{"swapped points", func(p *ReferencePlan) { p.Cases[29], p.Cases[30] = p.Cases[30], p.Cases[29] }},
		{"different answer", func(p *ReferencePlan) { p.Cases[30].Answer = p.Cases[29].Answer }},
		{"different source", func(p *ReferencePlan) { p.Source = p.Definition }},
		{"different upstream catalog", func(p *ReferencePlan) { p.Hashes = p.Definition }},
		{"different lock", func(p *ReferencePlan) { p.SourceLockSHA256 = p.Version }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := plan
			changed.Cases = slices.Clone(plan.Cases)
			tc.change(&changed)
			calls := 0
			s, err := changed.FreezeMaterials(t.Context(), func(context.Context, materialstore.Object) (io.ReadCloser, error) {
				calls++
				return nil, io.ErrUnexpectedEOF
			})
			if s != nil || !errors.Is(err, ErrReferenceUnsupported) || calls != 0 {
				t.Fatalf("expected rejection before material I/O, got %v, reads %d", err, calls)
			}
		})
	}
}

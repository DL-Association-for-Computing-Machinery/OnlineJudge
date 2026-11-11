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

func TestLocalReferenceRejectsChangedPlanBeforeMaterialReads(t *testing.T) {
	for version, revision := range localReferenceRevisions {
		t.Run(revision.sourceID, func(t *testing.T) {
			testChangedLocalPlan(t, version, revision)
		})
	}
}

func testChangedLocalPlan(t *testing.T, version string, revision localReferenceRevision) {
	t.Helper()
	first := revision.upstream
	cases := make([]ReferenceCase, first, first+len(revision.suffix))
	for i := range cases {
		cases[i] = ReferenceCase{ID: fmt.Sprintf("case-%04d", i+1), Input: revision.suffix[0].Input, Answer: revision.suffix[0].Answer}
	}
	var path string
	var deps []ReferenceDependency
	if revision.sourceID != "library-checker:aplusb" {
		a, err := referenceDiskAdmission(revision.sourceID)
		if err != nil {
			t.Fatal(err)
		}
		path = a.entry
		for _, dep := range a.dependencies {
			deps = append(deps, ReferenceDependency{Path: dep.target, Object: revision.source})
		}
	}
	plan := ReferencePlan{
		SourceID: revision.sourceID, SourceLockSHA256: localReferenceSourceLock, Version: version,
		ImportRevision: version, SourcePath: path, Source: revision.source,
		Definition: revision.definition, Hashes: revision.hashes, Dependencies: deps, Cases: append(cases, revision.suffix...),
	}
	if err := plan.ValidateMaterials(); err != nil {
		t.Fatal(err)
	}
	otherSource := "library-checker:unionfind"
	if revision.sourceID == otherSource {
		otherSource = "library-checker:zalgorithm"
	}
	swapWith := first + 1
	if len(revision.suffix) == 1 {
		swapWith = first - 1 // Move the sole local point into the upstream prefix.
	}
	for _, tc := range []struct {
		name   string
		change func(*ReferencePlan)
	}{
		{"omitted point", func(p *ReferencePlan) { p.Cases = p.Cases[:len(p.Cases)-1] }},
		{"swapped points", func(p *ReferencePlan) { p.Cases[first], p.Cases[swapWith] = p.Cases[swapWith], p.Cases[first] }},
		{"different input", func(p *ReferencePlan) { p.Cases[first].Input.SHA256 = p.Version }},
		{"different answer", func(p *ReferencePlan) { p.Cases[first].Answer.SHA256 = p.Version }},
		{"different problem", func(p *ReferencePlan) { p.SourceID = otherSource }},
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

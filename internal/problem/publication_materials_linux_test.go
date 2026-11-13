//go:build linux

package problem

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

const publicationFixtureID = "44444444444444444444444444444444"

func publicationFixture(t *testing.T, invalidImage bool) (previewFixture, Draft, ImportIdentity, ReferencePlan) {
	t.Helper()
	f := newPreviewFixture(t, func(b *problemfixture.Bundle) {
		b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", []byte("synthetic private reference marker\n"))
		cases := b.Document["test_cases"].([]any)
		private := func(raw string) map[string]any {
			digest := problemfixture.Hash([]byte(raw))
			path := "objects/sha256/" + digest
			b.Files[path] = []byte(raw)
			return map[string]any{"path": path, "sha256": digest, "size": len(raw)}
		}
		b.Document["test_cases"] = append(cases, map[string]any{"id": "case-0002", "sample": false,
			"input": private("synthetic hidden input\n"), "answer": private("synthetic hidden answer\n")})
		provenance := b.Document["provenance"].(map[string]any)
		provenance["source_case_mapping"] = append(provenance["source_case_mapping"].([]any),
			map[string]any{"case_id": "case-0002", "upstream_case": "random_00"})
		b.BindReferenceCases(t)
		image := append(problemfixture.PNG(t, 2, 3), []byte("synthetic private image trailer")...)
		if invalidImage {
			image = []byte("synthetic unsupported image")
		}
		b.AddStatementImage(t, "images/diagram.png", image)
		b.AddOriginal(t, "sample/aplusb/private-unused.txt", []byte("synthetic unselected file\n"))
	})
	d := f.repository.draft
	p, err := LoadBundle(f.bundle.Directory, d.SourceID, f.bundle.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closePrepared(t, p) })
	bound, err := p.ImportIdentity()
	if err != nil {
		t.Fatal(err)
	}
	d.ImportRevision = bound.ImportRevision
	plan, err := p.PrepareReference(t.Context(), SmallReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	plan.SourceData = nil
	for i := range plan.Cases {
		plan.Cases[i].InputData, plan.Cases[i].AnswerData = nil, nil
	}
	return f, d, bound, plan
}

func TestPublicationMaterialSelectionDemandsExactTrustedBinding(t *testing.T) {
	f, d, identity, plan := publicationFixture(t, false)
	if _, err := d.PlanPublicationMaterials(publicationFixtureID, d.ID, d.Owner, identity, plan, plan.Digest()); err != nil {
		t.Fatal(err)
	}
	other := plan
	other.Cases = slices.Clone(plan.Cases)
	other.Cases[1].Answer = other.Cases[0].Answer
	// Even a newly computed digest for the modified plan cannot make it match
	// the immutable import's independent source mapping.
	if _, err := d.PlanPublicationMaterials(publicationFixtureID, d.ID, d.Owner, identity, other, other.Digest()); err == nil {
		t.Fatal("mismatched private answer used as the formal plan")
	}
	for _, candidate := range []struct{ problem, expected string }{
		{d.ID, plan.Digest()}, {publicationFixtureID, strings.Repeat("b", 64)},
	} {
		if _, err := d.PlanPublicationMaterials(candidate.problem, d.ID, d.Owner, identity, plan, candidate.expected); err == nil {
			t.Fatal("wrong public identity or wrong independent pin was accepted")
		}
	}
	if len(f.materials.opened) != 0 {
		t.Fatal("metadata selection performed storage I/O")
	}
	selected, err := d.PlanPublicationMaterials(publicationFixtureID, d.ID, d.Owner, identity, plan, plan.Digest())
	if err != nil {
		t.Fatal(err)
	}
	got, raw, err := selected.statementBytes(t.Context(), f.local)
	if err != nil || got.ProblemID != publicationFixtureID || !bytes.Contains(raw, []byte("# Synthetic A + B")) ||
		bytes.Contains(raw, []byte("synthetic hidden")) {
		t.Fatalf("could not build the bounded display projection: %v", err)
	}
}

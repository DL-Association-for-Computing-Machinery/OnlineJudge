package problem_test

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func transportFixture(t *testing.T, b *problemfixture.Bundle) (problem.ReferencePlan, func(context.Context, materialstore.Object) (io.ReadCloser, error)) {
	t.Helper()
	provenance := b.Document["provenance"].(map[string]any)
	source := provenance["source_id"].(string)
	b.AddOriginal(t, provenance["source_path"].(string)+"/sol/correct.cpp", []byte("synthetic transport source"))
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, source, b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	}()
	var plan problem.ReferencePlan
	if source == "library-checker:aplusb" {
		plan, err = p.PrepareReference(t.Context(), problem.SmallReferenceLimits())
	} else {
		var s *problem.ReferenceSnapshot
		s, err = p.PrepareDiskReference(t.Context())
		if err == nil {
			plan = s.Plan()
			err = s.Close()
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	plan.SourceData = nil
	for i := range plan.Cases {
		plan.Cases[i].InputData = nil
		plan.Cases[i].AnswerData = nil
	}
	return plan, func(_ context.Context, o materialstore.Object) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(b.Files["objects/sha256/"+o.SHA256])), nil
	}
}

func TestTransportFreezesAllAdmittedSources(t *testing.T) {
	for _, factory := range []func(*testing.T) *problemfixture.Bundle{problemfixture.New, problemfixture.NewUnionFind, problemfixture.NewZAlgorithm, problemfixture.NewSuffixArray, problemfixture.NewStaticRMQ, problemfixture.NewPointAddRangeSum, problemfixture.NewStaticRangeSum, problemfixture.NewAssociativeArray, problemfixture.NewPrimality, problemfixture.NewConvolution} {
		b := factory(t)
		t.Run(b.Document["provenance"].(map[string]any)["source_id"].(string), func(t *testing.T) {
			plan, open := transportFixture(t, b)
			s, err := plan.FreezeMaterials(t.Context(), open)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := s.Close(); err != nil {
					t.Error(err)
				}
			}()
			if !reflect.DeepEqual(s.Plan(), plan) {
				t.Fatal("transport changed the canonical plan")
			}
			got, err := s.ReadSource(t.Context())
			if err != nil || string(got) != "synthetic transport source" {
				t.Fatal("source bytes changed", err)
			}
			deps, err := s.ReadDependencies(t.Context())
			if err != nil || len(deps) != len(plan.Dependencies) {
				t.Fatal("dependencies changed", err)
			}
			for i, c := range plan.Cases {
				in, answer, err := s.ReadCase(t.Context(), i)
				if err != nil || problemfixture.Hash(in) != c.Input.SHA256 || problemfixture.Hash(answer) != c.Answer.SHA256 {
					t.Fatal("case bytes changed", err)
				}
			}
		})
	}
}

func TestTransportRejectsBudgetAndPartialSnapshots(t *testing.T) {
	plan, open := transportFixture(t, problemfixture.NewUnionFind(t))
	for _, kind := range []string{"negative", "overflow", "source path", "case id", "private bytes", "extra dependency", "bad digest"} {
		t.Run(kind, func(t *testing.T) {
			bad := plan
			bad.Cases = append([]problem.ReferenceCase(nil), plan.Cases...)
			switch kind {
			case "negative":
				bad.Cases[0].Input.Size = -1
			case "overflow":
				bad.Cases[0].Input.Size = 1 << 62
			case "source path":
				bad.SourcePath = "untrusted"
			case "case id":
				bad.Cases[0].ID = "case-0002"
			case "private bytes":
				bad.SourceData = []byte("private")
			case "extra dependency":
				bad.Dependencies = []problem.ReferenceDependency{{Path: "untrusted"}}
			case "bad digest":
				bad.Source.SHA256 = "invalid"
			}
			calls := 0
			s, err := bad.FreezeMaterials(t.Context(), func(context.Context, materialstore.Object) (io.ReadCloser, error) {
				calls++
				return nil, io.ErrUnexpectedEOF
			})
			if s != nil || err == nil || calls != 0 {
				t.Fatal("invalid plan reached material I/O", err, calls)
			}
		})
	}
	for _, kind := range []string{"last corrupt", "last truncated", "last extra", "catalog"} {
		t.Run(kind, func(t *testing.T) {
			target := plan.Cases[len(plan.Cases)-1].Answer
			if kind == "catalog" {
				target = plan.Hashes
			}
			s, err := plan.FreezeMaterials(t.Context(), func(ctx context.Context, o materialstore.Object) (io.ReadCloser, error) {
				if o != target {
					return open(ctx, o)
				}
				f, e := open(ctx, o)
				if e != nil {
					return nil, e
				}
				raw, e := io.ReadAll(f)
				_ = f.Close()
				if e != nil {
					return nil, e
				}
				switch kind {
				case "last extra":
					raw = append(raw, 'x')
				case "last truncated":
					raw = raw[:len(raw)-1]
				default:
					raw[0] ^= 1
				}
				return io.NopCloser(bytes.NewReader(raw)), nil
			})
			if s != nil || err == nil {
				t.Fatal("partial snapshot escaped", err)
			}
			good, err := plan.FreezeMaterials(t.Context(), open)
			if err != nil {
				t.Fatal("failed transport retained quota", err)
			}
			if err := good.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTransportRejectsSubsetOrderAndDependencyDrift(t *testing.T) {
	b := problemfixture.NewUnionFind(t)
	input := []byte("different second point\n")
	answer := []byte("different second answer\n")
	ref := func(raw []byte) map[string]any {
		hash := problemfixture.Hash(raw)
		b.Files["objects/sha256/"+hash] = raw
		return map[string]any{"path": "objects/sha256/" + hash, "sha256": hash, "size": len(raw)}
	}
	b.Document["test_cases"] = append(b.Document["test_cases"].([]any), map[string]any{"id": "case-0002", "input": ref(input), "answer": ref(answer), "sample": false})
	provenance := b.Document["provenance"].(map[string]any)
	provenance["source_case_mapping"] = append(provenance["source_case_mapping"].([]any), map[string]any{"case_id": "case-0002", "upstream_case": "random_00"})
	plan, open := transportFixture(t, b)
	for _, kind := range []string{"subset", "reorder"} {
		t.Run(kind, func(t *testing.T) {
			bad := plan
			bad.Cases = append([]problem.ReferenceCase(nil), plan.Cases...)
			if kind == "subset" {
				bad.Cases = bad.Cases[:1]
			} else {
				bad.Cases[0], bad.Cases[1] = bad.Cases[1], bad.Cases[0]
				bad.Cases[0].ID = "case-0001"
				bad.Cases[1].ID = "case-0002"
			}
			if err := bad.ValidateMaterials(); err != nil {
				t.Fatal("test did not reach catalogue validation", err)
			}
			s, err := bad.FreezeMaterials(t.Context(), open)
			if s != nil || err == nil {
				t.Fatal("authentic catalogue admitted incomplete/reordered plan", err)
			}
			good, err := plan.FreezeMaterials(t.Context(), open)
			if err != nil {
				t.Fatal("catalogue rejection retained quota", err)
			}
			if err := good.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
	dependencyPlan, _ := transportFixture(t, problemfixture.NewPrimality(t))
	for _, kind := range []string{"rename", "reorder", "omit"} {
		bad := dependencyPlan
		bad.Dependencies = append([]problem.ReferenceDependency(nil), bad.Dependencies...)
		switch kind {
		case "rename":
			bad.Dependencies[0].Path = "arbitrary.hpp"
		case "reorder":
			bad.Dependencies[0], bad.Dependencies[1] = bad.Dependencies[1], bad.Dependencies[0]
		case "omit":
			bad.Dependencies = bad.Dependencies[:1]
		}
		if err := bad.ValidateMaterials(); err == nil {
			t.Fatal("dependency closure drift admitted", kind)
		}
	}
}

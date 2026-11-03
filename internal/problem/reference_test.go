package problem_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func referenceFixture(t *testing.T) (*problemfixture.Bundle, *problem.Prepared) {
	t.Helper()
	b := problemfixture.New(t)
	b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", []byte("int main(){}"))
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	})
	return b, p
}

func TestReferenceSnapshotAndLimits(t *testing.T) {
	b, p := referenceFixture(t)
	limits := problemvalidation.PinnedProfile().Materials
	plan, err := p.PrepareReference(context.Background(), limits)
	if err != nil || len(plan.Cases) != 1 || string(plan.Cases[0].InputData) != "1 2\n" || plan.ImportRevision != p.Content().RequestSHA256 {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	before := plan.Digest()
	plan.Cases[0].InputData[0] = '9'
	other, err := p.PrepareReference(context.Background(), limits)
	if err != nil || other.Cases[0].InputData[0] != '1' || other.Digest() != before {
		t.Fatal("snapshot aliases internal state")
	}
	raw, _ := json.Marshal(other)
	if strings.Contains(string(raw), "int main") || strings.Contains(string(raw), "1 2\\n") {
		t.Fatal("plan serialization leaked private bytes")
	}
	limits.SourceBytes = 1
	if _, err := p.PrepareReference(context.Background(), limits); !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatalf("source overflow: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.PrepareReference(ctx, problemvalidation.PinnedProfile().Materials); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	// Corrupting stored bytes later cannot mutate an already frozen plan.
	path := filepath.Join(b.Directory, "objects/sha256/"+other.Cases[0].Answer.SHA256)
	if err := os.WriteFile(path, []byte("4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if string(other.Cases[0].AnswerData) != "3\n" {
		t.Fatal("frozen bytes changed")
	}
	if _, err := p.PrepareReference(context.Background(), problemvalidation.PinnedProfile().Materials); err == nil {
		t.Fatal("corrupt reference admitted")
	}
}

func TestReferenceRejectsNonASCIIAndMissingSource(t *testing.T) {
	for _, role := range []string{"source", "input", "answer", "missing"} {
		t.Run(role, func(t *testing.T) {
			b := problemfixture.New(t)
			if role != "missing" {
				code := []byte("int main(){}")
				if role == "source" {
					code = []byte("//中文")
				}
				b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", code)
			}
			if role == "input" || role == "answer" {
				c := b.Document["test_cases"].([]any)[0].(map[string]any)
				raw := []byte{0xff, '\n'}
				c[role] = map[string]any{"path": "objects/sha256/" + problemfixture.Hash(raw), "size": len(raw), "sha256": problemfixture.Hash(raw)}
				b.Files["objects/sha256/"+problemfixture.Hash(raw)] = raw
			}
			b.BindReferenceCases(t)
			b.Write(t)
			p, err := problem.LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = p.Close() }()
			if _, err := p.PrepareReference(context.Background(), problemvalidation.PinnedProfile().Materials); !errors.Is(err, problem.ErrReferenceUnsupported) {
				t.Fatalf("%s admitted: %v", role, err)
			}
		})
	}
}

func TestReferenceRejectsIncompleteSource(t *testing.T) {
	b := problemfixture.New(t)
	b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", []byte("int main(){}"))
	c := b.Document["test_cases"].([]any)[0].(map[string]any)
	b.Document["test_cases"] = append(b.Document["test_cases"].([]any), map[string]any{"id": "case-0002", "input": c["input"], "answer": c["answer"], "sample": false})
	p := b.Document["provenance"].(map[string]any)
	p["source_case_mapping"] = append(p["source_case_mapping"].([]any), map[string]any{"case_id": "case-0002", "upstream_case": "random_00"})
	b.BindReferenceCases(t)
	// Keep the independently pinned TWO-point source; re-version only the draft.
	b.Document["test_cases"] = b.Document["test_cases"].([]any)[:1]
	p["source_case_mapping"] = p["source_case_mapping"].([]any)[:1]
	b.Write(t)
	prepared, err := problem.LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
	if err != nil {
		t.Fatalf("fixture must pass ordinary bundle integrity first: %v", err)
	}
	defer func() { _ = prepared.Close() }()
	if _, err := prepared.PrepareReference(context.Background(), problemvalidation.PinnedProfile().Materials); !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatalf("incomplete source admitted: %v", err)
	}
}

func TestReferenceSourceBindingFailures(t *testing.T) {
	for _, scenario := range []string{"input-hash", "answer-hash", "order", "sample", "extra-hash", "duplicate-hash", "group-count"} {
		t.Run(scenario, func(t *testing.T) {
			b := problemfixture.New(t)
			b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", []byte("int main(){}"))
			c := b.Document["test_cases"].([]any)[0].(map[string]any)
			b.Document["test_cases"] = append(b.Document["test_cases"].([]any), map[string]any{"id": "case-0002", "input": c["input"], "answer": c["answer"], "sample": false})
			p := b.Document["provenance"].(map[string]any)
			p["source_case_mapping"] = append(p["source_case_mapping"].([]any), map[string]any{"case_id": "case-0002", "upstream_case": "random_00"})
			b.BindReferenceCases(t)
			switch scenario {
			case "input-hash":
				c["input"] = c["answer"]
			case "answer-hash":
				c["answer"] = c["input"]
			case "order":
				m := p["source_case_mapping"].([]any)
				m[0].(map[string]any)["upstream_case"] = "random_00"
				m[1].(map[string]any)["upstream_case"] = "example_00"
			case "sample":
				b.Document["test_cases"].([]any)[1].(map[string]any)["sample"] = true
				b.Document["examples"] = []string{"case-0001", "case-0002"}
			case "extra-hash", "duplicate-hash":
				ref := p["original_files"].(map[string]any)["source/sample/aplusb/hash.json"].(map[string]any)
				raw := b.Files[ref["path"].(string)]
				var hashes map[string]string
				if err := json.Unmarshal(raw, &hashes); err != nil {
					t.Fatal(err)
				}
				if scenario == "extra-hash" {
					hashes["extra.in"] = hashes["example_00.in"]
					raw = problemfixture.JSON(t, hashes)
				} else {
					raw = []byte(`{"example_00.in":"` + hashes["example_00.in"] + `",` + strings.TrimSpace(string(raw))[1:])
				}
				b.AddOriginal(t, "sample/aplusb/hash.json", raw)
			case "group-count":
				b.AddOriginal(t, "sample/aplusb/info.toml", []byte("[[tests]]\nname='example.in'\nnumber=0\n"))
			}
			b.Write(t)
			prepared, err := problem.LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
			if err != nil {
				t.Fatalf("must pass ordinary version/index checks first: %v", err)
			}
			defer func() { _ = prepared.Close() }()
			if _, err := prepared.PrepareReference(context.Background(), problemvalidation.PinnedProfile().Materials); !errors.Is(err, problem.ErrReferenceUnsupported) {
				t.Fatalf("%s source binding admitted: %v", scenario, err)
			}
		})
	}
}

func TestReferenceDefinitionOrderIsNotHashDictionaryOrder(t *testing.T) {
	b := problemfixture.New(t)
	b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", []byte("int main(){}"))
	first := b.Document["test_cases"].([]any)[0].(map[string]any)
	b.Document["test_cases"] = append(b.Document["test_cases"].([]any), map[string]any{"id": "case-0002", "input": first["input"], "answer": first["answer"], "sample": false})
	p := b.Document["provenance"].(map[string]any)
	p["source_case_mapping"] = append(p["source_case_mapping"].([]any), map[string]any{"case_id": "case-0002", "upstream_case": "random_00"})
	b.BindReferenceCases(t)
	b.AddOriginal(t, "sample/aplusb/info.toml", []byte("[[tests]]\nname='random.cpp'\nnumber=1\n[[tests]]\nname='example.in'\nnumber=1\n"))
	mapping := p["source_case_mapping"].([]any)
	mapping[0].(map[string]any)["upstream_case"] = "random_00"
	mapping[1].(map[string]any)["upstream_case"] = "example_00"
	first["sample"] = false
	b.Document["test_cases"].([]any)[1].(map[string]any)["sample"] = true
	b.Document["examples"] = []string{"case-0002"}
	b.Write(t)
	prepared, err := problem.LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = prepared.Close() }()
	if _, err := prepared.PrepareReference(context.Background(), problemvalidation.PinnedProfile().Materials); err != nil {
		t.Fatalf("TOML group order rejected: %v", err)
	}
}

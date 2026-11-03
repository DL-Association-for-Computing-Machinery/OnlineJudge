package problemvalidation_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func unionRunnerFixture(t *testing.T, lastAnswer []byte) (*problemfixture.Bundle, *problem.Prepared) {
	t.Helper()
	b := problemfixture.NewUnionFind(t)
	b.AddOriginal(t, "data_structure/unionfind/sol/correct.cpp", []byte("int main(){}"))
	second := map[string]any{"id": "case-0002", "sample": false}
	for role, raw := range map[string][]byte{"input": []byte("2 3\n"), "answer": lastAnswer} {
		sha := problemfixture.Hash(raw)
		second[role] = map[string]any{"path": "objects/sha256/" + sha, "sha256": sha, "size": len(raw)}
		b.Files["objects/sha256/"+sha] = raw
	}
	b.Document["test_cases"] = append(b.Document["test_cases"].([]any), second)
	p := b.Document["provenance"].(map[string]any)
	p["source_case_mapping"] = append(p["source_case_mapping"].([]any), map[string]any{"case_id": "case-0002", "upstream_case": "random_00"})
	b.BindReferenceCases(t)
	b.Write(t)
	prepared, err := problem.LoadBundle(b.Directory, "library-checker:unionfind", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := prepared.Close(); err != nil {
			t.Error(err)
		}
	})
	return b, prepared
}

func TestUnionFindPreflightBeforeExecutionAndSnapshotIsolation(t *testing.T) {
	for _, scenario := range []string{"frozen", "WA", "cleanup", "bad-utf8", "bad-token", "corrupt-late"} {
		t.Run(scenario, func(t *testing.T) {
			answer := []byte("5\n")
			if scenario == "bad-utf8" {
				answer = []byte{255, '\n'}
			}
			if scenario == "bad-token" {
				answer = []byte(strings.Repeat("x", 4097))
			}
			b, p := unionRunnerFixture(t, answer)
			s := &sandbox{runtime: experiment.PinnedReferenceEnvironment(), compile: accepted(""), run: []execution.Observation{accepted("3"), accepted("5")}}
			s.compile.Artifact = "compiled"
			ref := b.Document["test_cases"].([]any)[1].(map[string]any)["answer"].(map[string]any)
			mutate := func() {
				if err := os.WriteFile(filepath.Join(b.Directory, ref["path"].(string)), []byte("9\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "frozen" {
				s.onVerify = mutate
			}
			if scenario == "corrupt-late" {
				mutate()
			}
			if scenario == "WA" {
				s.run[0] = accepted("wrong")
			}
			if scenario == "cleanup" {
				s.removeErr = errors.New("cleanup")
			}
			r, err := problemvalidation.New(s).ValidateUnionFind(context.Background(), p, problemvalidation.UnionFindProfile())
			if strings.HasPrefix(scenario, "bad-") || scenario == "corrupt-late" {
				if err == nil || len(s.calls) != 0 {
					t.Fatal("late bad reference reached sandbox", err)
				}
				return
			}
			want := "AC"
			if scenario == "WA" {
				want = "WA"
			}
			if scenario == "cleanup" {
				want = "SE"
			}
			if r.Verdict != want || (err != nil) != (scenario == "cleanup") {
				t.Fatal(r.Verdict, err)
			}
			if scenario == "WA" && r.Points[1].Verdict != "NOT_RUN" {
				t.Fatal("first failure did not stop")
			}
			if scenario == "frozen" && len(s.calls) != 5 {
				t.Fatal("not all points executed", s.calls)
			}
			if e := r.Finalize(err, nil, nil); (e != nil) != (scenario == "cleanup") {
				t.Fatal(e)
			}
			raw, e := json.Marshal(r)
			if e != nil || strings.Contains(string(raw), "int main()") || strings.Contains(string(raw), "InputData") {
				t.Fatal("private snapshot leaked")
			}
		})
	}
}

func TestUnionFindProfileCannotReachOtherEntry(t *testing.T) {
	_, p := unionRunnerFixture(t, []byte("5\n"))
	s := &sandbox{}
	if _, err := problemvalidation.New(s).Validate(context.Background(), p, problemvalidation.UnionFindProfile()); err == nil || len(s.calls) != 0 {
		t.Fatal("old entry widened")
	}
	if _, err := problemvalidation.New(s).ValidateUnionFind(context.Background(), p, problemvalidation.PinnedProfile()); err == nil || len(s.calls) != 0 {
		t.Fatal("reference entry accepted M0")
	}
	profile := problemvalidation.UnionFindProfile()
	before := profile.Digest()
	profile.Runtime.OutputBytes++
	if profile.Digest() == before {
		t.Fatal("profile digest omitted runtime")
	}
	if _, err := problemvalidation.New(s).ValidateUnionFind(context.Background(), p, profile); err == nil || len(s.calls) != 0 {
		t.Fatal("arbitrary reference runtime accepted")
	}
}

package problemvalidation_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

type sandbox struct {
	calls             []string
	compile           execution.Observation
	run               []execution.Observation
	removeErr, runErr error
	cancel            context.CancelFunc
	removeCancel      context.CancelFunc
}

func (s *sandbox) Verify(_ context.Context, p experiment.ExecutionProfile) error {
	s.calls = append(s.calls, "verify")
	if !reflect.DeepEqual(p, experiment.PinnedEnvironment()) {
		return errors.New("runtime")
	}
	return nil
}
func (s *sandbox) Compile(_ context.Context, _ experiment.ExecutionProfile, _ string) (execution.Observation, error) {
	s.calls = append(s.calls, "compile")
	return s.compile, nil
}
func (s *sandbox) Run(_ context.Context, _ experiment.ExecutionProfile, _ string, input []byte) (execution.Observation, error) {
	s.calls = append(s.calls, "run:"+string(input))
	if s.cancel != nil {
		s.cancel()
	}
	if s.runErr != nil {
		return execution.Observation{}, s.runErr
	}
	o := s.run[0]
	s.run = s.run[1:]
	return o, nil
}
func (s *sandbox) Remove(ctx context.Context, _ string) error {
	s.calls = append(s.calls, "remove")
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if s.removeCancel != nil {
		s.removeCancel()
	}
	return s.removeErr
}
func accepted(out string) execution.Observation {
	return execution.Observation{Status: "Accepted", Stdout: []byte(out), CPU: 1, Memory: 1}
}

func fixture(t *testing.T) (*problemfixture.Bundle, *problem.Prepared) {
	t.Helper()
	b := problemfixture.New(t)
	b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", []byte("int main(){}"))
	second := map[string]any{"id": "case-0002", "sample": false}
	for role, raw := range map[string][]byte{"input": []byte("2 3\n"), "answer": []byte("5\n")} {
		sha := problemfixture.Hash(raw)
		second[role] = map[string]any{"path": "objects/sha256/" + sha, "sha256": sha, "size": len(raw)}
		b.Files["objects/sha256/"+sha] = raw
	}
	b.Document["test_cases"] = append(b.Document["test_cases"].([]any), second)
	p := b.Document["provenance"].(map[string]any)
	p["source_case_mapping"] = append(p["source_case_mapping"].([]any), map[string]any{"case_id": "case-0002", "upstream_case": "random_00"})
	b.BindReferenceCases(t)
	b.Write(t)
	prepared, err := problem.LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Close() })
	return b, prepared
}

func TestReferenceRunnerAndFailureBoundaries(t *testing.T) {
	for _, name := range []string{"AC", "WA", "CE", "TLE", "OLE", "cleanup", "run-error", "cancel", "cancel-remove"} {
		t.Run(name, func(t *testing.T) {
			_, p := fixture(t)
			s := &sandbox{compile: accepted("")}
			s.compile.Artifact = "compiled"
			s.run = []execution.Observation{accepted("3\n"), accepted("5\n")}
			want := "AC"
			wantErr := false
			switch name {
			case "WA":
				s.run[0] = accepted("4")
				want = "WA"
			case "CE":
				s.compile = execution.Observation{Status: "Nonzero Exit Status", Exit: 1}
				want = "CE"
			case "TLE":
				s.run[0] = execution.Observation{Status: "Time Limit Exceeded"}
				want = "TLE"
			case "OLE":
				s.run[0] = execution.Observation{Status: "Output Limit Exceeded", Exit: 25}
				want = "OLE"
			case "cleanup":
				s.removeErr = errors.New("cleanup")
				want = "SE"
				wantErr = true
			case "run-error":
				s.runErr = errors.New("transport")
				want = "SE"
				wantErr = true
			case "cancel", "cancel-remove":
				want = "SE"
				wantErr = true
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if name == "cancel" {
				s.cancel = cancel
			}
			if name == "cancel-remove" {
				s.removeCancel = cancel
			}
			runner := problemvalidation.New(s)
			report, err := runner.Validate(ctx, p, problemvalidation.PinnedProfile())
			if (err != nil) != wantErr || report.Verdict != want || report.Status == "reference-validation-passed" || report.PlanSHA256 == "" {
				t.Fatalf("%+v err=%v", report, err)
			}
			if err := report.Finalize(err, nil, nil); (err != nil) != wantErr {
				t.Fatal(err)
			}
			if (report.Status == "reference-validation-passed") != (name == "AC") {
				t.Fatal("incorrect finalized status")
			}
			if name != "AC" && name != "cleanup" && name != "cancel-remove" && report.Points[1].Verdict != "NOT_RUN" {
				t.Fatal("late point executed")
			}
			if name != "CE" && s.calls[len(s.calls)-1] != "remove" {
				t.Fatal("artifact not cleaned")
			}
			wantCalls := []string{"verify", "compile"}
			if name != "CE" {
				wantCalls = append(wantCalls, "run:1 2\n")
				if name == "AC" || name == "cleanup" || name == "cancel-remove" {
					wantCalls = append(wantCalls, "run:2 3\n")
				}
				wantCalls = append(wantCalls, "remove")
			}
			if !reflect.DeepEqual(s.calls, wantCalls) {
				t.Fatalf("compile/order/early-stop calls: %v", s.calls)
			}
			if _, err := runner.Validate(context.Background(), p, problemvalidation.PinnedProfile()); err == nil {
				t.Fatal("runner reused")
			}
		})
	}
}

func TestAllReferencesPrecedeSandboxAndProfileBinding(t *testing.T) {
	b, p := fixture(t)
	s := &sandbox{}
	answer := b.Document["test_cases"].([]any)[1].(map[string]any)["answer"].(map[string]any)
	if err := os.WriteFile(filepath.Join(b.Directory, answer["path"].(string)), []byte("4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := problemvalidation.New(s).Validate(context.Background(), p, problemvalidation.PinnedProfile())
	if err == nil || len(s.calls) != 0 || len(report.Points) != 2 || report.PlanSHA256 != "" || report.Plan.Version != p.Content().Version {
		t.Fatal("corrupt later reference reached sandbox")
	}
	profile := problemvalidation.PinnedProfile()
	original := profile.Digest()
	profile.Runtime.Env[0] = "untrusted"
	if profile.Digest() == original || problemvalidation.PinnedProfile().Runtime.Env[0] == "untrusted" {
		t.Fatal("profile hash/snapshot")
	}
	if _, err := problemvalidation.New(s).Validate(context.Background(), p, profile); err == nil || len(s.calls) != 0 {
		t.Fatal("altered profile admitted")
	}
	for _, field := range []string{"stop", "clean"} {
		report := problemvalidation.Report{Verdict: "AC"}
		var stop, clean error
		if field == "stop" {
			stop = errors.New("unknown")
		} else {
			clean = errors.New("sync")
		}
		if report.Finalize(nil, stop, clean) == nil || report.Status == "reference-validation-passed" || report.Verdict != "SE" {
			t.Fatal("unconfirmed cleanup passed")
		}
	}
}

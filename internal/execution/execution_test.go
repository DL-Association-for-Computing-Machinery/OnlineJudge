package execution

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

func fixture() (materials.Descriptor, map[string][]byte) {
	v, m, _ := experiment.SelectFixture(experiment.PinnedVersion)
	source := "int main(){}"
	d := materials.Descriptor{TaskMaterials: submission.TaskMaterials{Version: v, Manifest: m, ManifestSHA256: digest(m), Language: "gnu++17", Source: source}, SourceSHA256: digest([]byte(source))}
	blobs := map[string][]byte{}
	for _, s := range []string{"1 2\n", "3\n", "-7 4\n", "-3\n"} {
		blobs[digest([]byte(s))] = []byte(s)
	}
	return d, blobs
}

type fakeSandbox struct {
	compiled                                 Observation
	runs                                     []Observation
	calls                                    []string
	verifyErr, compileErr, runErr, removeErr error
	onRun                                    func()
}

func goodSandbox() *fakeSandbox {
	return &fakeSandbox{compiled: Observation{Status: "Accepted", Artifact: "artifact", CPU: 100, Memory: 1000}, runs: []Observation{{Status: "Accepted", Stdout: []byte("3\n"), CPU: 10, Memory: 30}, {Status: "Accepted", Stdout: []byte("-3\n"), CPU: 20, Memory: 25}}}
}
func (s *fakeSandbox) Verify(ctx context.Context, p experiment.ExecutionProfile) error {
	s.calls = append(s.calls, "verify")
	return s.verifyErr
}
func (s *fakeSandbox) Compile(ctx context.Context, p experiment.ExecutionProfile, source string) (Observation, error) {
	s.calls = append(s.calls, "compile")
	return s.compiled, s.compileErr
}
func (s *fakeSandbox) Run(ctx context.Context, p experiment.ExecutionProfile, id string, input []byte) (Observation, error) {
	s.calls = append(s.calls, string(input))
	if s.onRun != nil {
		s.onRun()
	}
	if s.runErr != nil {
		return Observation{}, s.runErr
	}
	o := s.runs[0]
	s.runs = s.runs[1:]
	return o, nil
}
func (s *fakeSandbox) Remove(ctx context.Context, id string) error {
	s.calls = append(s.calls, "remove:"+id)
	return s.removeErr
}

func TestOrderedAttempt(t *testing.T) {
	for _, fail := range []int{-1, 0, 1} {
		t.Run(map[int]string{-1: "AC", 0: "first WA", 1: "last WA"}[fail], func(t *testing.T) {
			d, b := fixture()
			s := goodSandbox()
			want := "AC"
			if fail >= 0 {
				s.runs[fail].Stdout = []byte("wrong")
				want = "WA"
			}
			r, err := New(s).Judge(context.Background(), d, b)
			if err != nil || r.Verdict != want {
				t.Fatalf("%+v %v", r, err)
			}
			calls := []string{"verify", "compile", "1 2\n"}
			if fail != 0 {
				calls = append(calls, "-7 4\n")
			}
			calls = append(calls, "remove:artifact")
			if !reflect.DeepEqual(s.calls, calls) {
				t.Fatalf("calls: %q", s.calls)
			}
			if fail == 0 {
				if r.Points[1].Verdict != "NOT_RUN" || r.Points[1].Usage != nil || r.Max == nil || *r.Max != (Usage{10, 30}) {
					t.Fatalf("unexecuted stats: %+v", r)
				}
			} else if r.Max == nil || *r.Max != (Usage{20, 30}) {
				t.Fatalf("maxima: %+v", r.Max)
			}
			if r.Compile == nil || r.Compile.CPU != 100 {
				t.Fatal("compilation not separate")
			}
		})
	}
}

func TestRejectMaterialBeforeSandbox(t *testing.T) {
	for _, name := range []string{"legacy", "source hash", "manifest hash", "unknown profile", "missing blob", "corrupt later answer", "oversized source", "empty source", "wrong language"} {
		t.Run(name, func(t *testing.T) {
			d, b := fixture()
			s := goodSandbox()
			switch name {
			case "legacy":
				d.Version = experiment.Version
			case "source hash":
				d.Source += " "
			case "manifest hash":
				d.ManifestSHA256 = "wrong"
			case "unknown profile":
				d.Manifest = append(d.Manifest, ' ')
				d.ManifestSHA256 = digest(d.Manifest)
			case "missing blob":
				delete(b, digest([]byte("1 2\n")))
			case "corrupt later answer":
				b[digest([]byte("-3\n"))] = []byte("incorrect")
			case "oversized source":
				d.Source = string(make([]byte, submission.MaxSourceBytes+1))
				d.SourceSHA256 = digest([]byte(d.Source))
			case "empty source":
				d.Source = ""
				d.SourceSHA256 = digest(nil)
			case "wrong language":
				d.Language = "python"
			}
			r, err := New(s).Judge(context.Background(), d, b)
			if err == nil || r.Verdict != "SE" || len(s.calls) != 0 {
				t.Fatalf("executed invalid materials: %+v %v %q", r, err, s.calls)
			}
		})
	}
}

func TestOutcomeClassification(t *testing.T) {
	for _, tc := range []struct {
		status       string
		exit         int
		compile, run string
	}{
		{"Accepted", 0, "AC", "AC"}, {"Nonzero Exit Status", 1, "CE", "RE"},
		{"Time Limit Exceeded", 9, "CE", "TLE"}, {"Memory Limit Exceeded", 9, "CE", "MLE"}, {"Output Limit Exceeded", 9, "CE", "OLE"},
		{"Signalled", 11, "SE", "RE"}, {"Dangerous Syscall", 31, "SE", "RE"},
		{"Internal Error", 0, "SE", "SE"}, {"File Error", 0, "SE", "SE"}, {"unknown", 0, "SE", "SE"},
		{"Accepted", 1, "SE", "SE"}, {"Nonzero Exit Status", 0, "SE", "SE"},
	} {
		for _, compile := range []bool{true, false} {
			want := tc.run
			if compile {
				want = tc.compile
			}
			got, err := classify(Observation{Status: tc.status, Exit: tc.exit}, compile)
			if got != want || (err != nil) != (want == "SE") {
				t.Errorf("%s compile=%t got %s %v", tc.status, compile, got, err)
			}
		}
	}
}

func TestFailureStopsAndCleans(t *testing.T) {
	for _, name := range []string{"environment", "CE", "missing artifact", "compile transport", "run transport", "unknown result", "negative stats", "cleanup"} {
		t.Run(name, func(t *testing.T) {
			d, b := fixture()
			s := goodSandbox()
			failure := errors.New("unavailable")
			want := "SE"
			switch name {
			case "environment":
				s.verifyErr = failure
			case "CE":
				s.compiled.Status = "Nonzero Exit Status"
				s.compiled.Exit = 1
				want = "CE"
			case "missing artifact":
				s.compiled.Artifact = ""
			case "compile transport":
				s.compileErr = failure
			case "run transport":
				s.runErr = failure
			case "unknown result":
				s.runs[0].Status = "unexpected"
			case "negative stats":
				s.runs[0].CPU = -1
			case "cleanup":
				s.removeErr = failure
			}
			runner := New(s)
			r, err := runner.Judge(context.Background(), d, b)
			if r.Verdict != want || (err != nil) != (want == "SE") {
				t.Fatalf("%+v %v", r, err)
			}
			if name == "CE" && r.Max != nil {
				t.Fatal("invented execution statistics for CE")
			}
			if name != "environment" && name != "missing artifact" && s.calls[len(s.calls)-1] != "remove:artifact" {
				t.Fatal("artifact leaked")
			}
			if name != "environment" && name != "CE" {
				before := len(s.calls)
				_, err = runner.Judge(context.Background(), d, b)
				if err == nil || len(s.calls) != before {
					t.Fatal("unhealthy sandbox reused")
				}
			}
		})
	}
}

func TestCancellationAndCapacity(t *testing.T) {
	d, b := fixture()
	s := goodSandbox()
	runner := New(s)
	ctx, cancel := context.WithCancel(context.Background())
	s.onRun = func() {
		if _, err := runner.Judge(context.Background(), d, b); !errors.Is(err, ErrBusy) {
			t.Errorf("concurrent execution admitted: %v", err)
		}
		cancel()
	}
	r, err := runner.Judge(ctx, d, b)
	if !errors.Is(err, context.Canceled) || r.Verdict != "SE" || r.Points[1].Verdict != "NOT_RUN" {
		t.Fatalf("%+v %v", r, err)
	}
	if s.calls[len(s.calls)-1] != "remove:artifact" {
		t.Fatal("cancelled cleanup skipped")
	}
	s = goodSandbox()
	_, err = New(s).Judge(ctx, d, b)
	if !errors.Is(err, context.Canceled) || len(s.calls) != 0 {
		t.Fatal("cancelled attempt started")
	}
}

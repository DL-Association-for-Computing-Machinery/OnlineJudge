package problemvalidation_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

type failingEvidenceSandbox struct {
	sandbox
	compileErr     error
	runs           int
	cancelAfterRun context.CancelFunc
}

func (s *failingEvidenceSandbox) Compile(ctx context.Context, p experiment.ExecutionProfile, source string) (execution.Observation, error) {
	o, _ := s.sandbox.Compile(ctx, p, source)
	return o, s.compileErr
}

func (s *failingEvidenceSandbox) Run(ctx context.Context, p experiment.ExecutionProfile, id string, input []byte) (execution.Observation, error) {
	s.runs++
	if s.runs == 2 {
		if s.cancelAfterRun != nil {
			s.cancelAfterRun()
		}
		return execution.Observation{}, context.DeadlineExceeded
	}
	return s.sandbox.Run(ctx, p, id, input)
}

func TestRunnerExecutionEvidenceDefersAndPoint(t *testing.T) {
	for _, compileFailure := range []bool{false, true} {
		_, prepared := fixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		s := &failingEvidenceSandbox{sandbox: sandbox{compile: accepted(""), run: []execution.Observation{accepted("3\n")}, removeErr: errors.New("PRIVATE-cleanup")}, cancelAfterRun: cancel}
		s.compile.Artifact = "compiled"
		if compileFailure {
			s.compileErr = context.DeadlineExceeded
			s.removeCancel = cancel
		}
		report, err := problemvalidation.New(s).Validate(ctx, prepared, problemvalidation.PinnedProfile())
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, context.Canceled) || report.Verdict != "SE" {
			t.Fatal("existing error/defer contract changed")
		}
		op, point := "run", 2
		if compileFailure {
			op, point = "compile", 0
		}
		want := []problemvalidation.ExecutionFailure{
			{Operation: op, Point: point, Phase: "unknown", Role: "unknown", Code: "deadline"},
			{Operation: "cleanup", Phase: "unknown", Role: "unknown", Code: "unknown"},
			{Operation: "validation", Phase: "unknown", Role: "unknown", Code: "canceled"},
		}
		if report.ExecutionDiagnostic == nil || report.ExecutionDiagnostic.Truncated || !reflect.DeepEqual(report.ExecutionDiagnostic.Entries, want) {
			t.Fatalf("runner call/defer evidence: %+v", report.ExecutionDiagnostic)
		}
		if !compileFailure && (report.Points[0].Verdict != "AC" || report.Points[1].Verdict != "SE") {
			t.Fatal("point evidence detached from failure")
		}
	}
}

package problemvalidation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

type differentialSandbox struct {
	role              string
	log               *[]string
	runs              int
	wrongAt           int
	cleanupFails      bool
	outputMalformedAt int
}

func (s *differentialSandbox) Verify(context.Context, experiment.ExecutionProfile) error {
	*s.log = append(*s.log, "verify "+s.role)
	return nil
}

func (s *differentialSandbox) Compile(_ context.Context, _ experiment.ExecutionProfile, source string) (execution.Observation, error) {
	*s.log = append(*s.log, "compile "+source)
	return execution.Observation{Status: "Accepted", Artifact: s.role}, nil
}

func (s *differentialSandbox) Run(_ context.Context, _ experiment.ExecutionProfile, artifact string, input []byte) (execution.Observation, error) {
	if artifact != s.role {
		return execution.Observation{}, errors.New("wrong test artifact")
	}
	s.runs++
	if s.runs == s.outputMalformedAt {
		return execution.Observation{Status: "Accepted", Stdout: []byte("not integer")}, nil
	}
	var n, m int
	r := bytes.NewReader(input)
	if _, err := fmt.Fscan(r, &n, &m); err != nil {
		return execution.Observation{}, err
	}
	a, b := make([]uint64, n), make([]uint64, m)
	for _, row := range [][]uint64{a, b} {
		for i := range row {
			if _, err := fmt.Fscan(r, &row[i]); err != nil {
				return execution.Observation{}, err
			}
		}
	}
	out := make([]uint64, n+m-1)
	for i, x := range a {
		for j, y := range b {
			out[i+j] = (out[i+j] + x*y) % convolutionModulus
		}
	}
	if s.runs == s.wrongAt {
		out[0] = (out[0] + 1) % convolutionModulus
	}
	var raw bytes.Buffer
	for _, v := range out {
		fmt.Fprintf(&raw, "%d ", v)
	}
	return execution.Observation{Status: "Accepted", Stdout: raw.Bytes()}, nil
}

func (s *differentialSandbox) Remove(_ context.Context, artifact string) error {
	*s.log = append(*s.log, "remove "+artifact)
	if s.cleanupFails {
		return errors.New("test cleanup unknown")
	}
	return nil
}

func TestSmallConvolutionDifferentialFailsClosedAndKeepsFirstMismatch(t *testing.T) {
	for _, tc := range []struct {
		name, role, stage             string
		wrong, malformed, equal, runs int
		oracleCleanup                 bool
	}{
		{name: "matches", equal: 24, runs: 24},
		{name: "valid disagreement", role: "candidate", stage: "compare", wrong: 10, equal: 9, runs: 10},
		{name: "malformed is not a disagreement", role: "candidate", stage: "output", malformed: 4, equal: 3, runs: 4},
		{name: "oracle cleanup fences candidate", role: "oracle", stage: "cleanup", oracleCleanup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var order []string
			oracle := &differentialSandbox{role: "oracle", log: &order, cleanupFails: tc.oracleCleanup}
			candidate := &differentialSandbox{role: "candidate", log: &order, wrongAt: tc.wrong, outputMalformedAt: tc.malformed}
			factories := 0
			src := ConvolutionSources{
				revision:  "test",
				oracle:    convolutionProgram{source: "oracle", identity: ConvolutionProgram{SHA256: "oracle-id"}},
				candidate: convolutionProgram{source: "candidate", identity: ConvolutionProgram{SHA256: "candidate-id"}},
			}
			if tc.wrong != 0 {
				src.draft = ConvolutionDraftBinding{ID: "draft-id", Version: "version", ImportRevision: "import-revision"}
			}
			r, err := RunSmallConvolution(t.Context(), src, func() (execution.Sandbox, error) {
				factories++
				if factories == 1 {
					return oracle, nil
				}
				return candidate, nil
			})
			if (tc.stage != "") != (err != nil) || r.FailureRole != tc.role || r.FailureStage != tc.stage ||
				oracle.runs != 24 || r.OracleParsed != 24 || candidate.runs != tc.runs || r.CandidateEqual != tc.equal ||
				r.Status != "incomplete" || r.Publication != "blocked" || r.StopConfirmed {
				t.Fatalf("unexpected report or run counts: %+v, %v, candidate=%d", r, err, candidate.runs)
			}
			if tc.wrong != 0 {
				if r.Draft == nil || *r.Draft != src.draft {
					t.Fatal("counterexample lost the bound draft revision")
				}
				c := r.Counterexample
				if err != ErrConvolutionMismatch || c == nil || c.Case != 10 || c.Input != "2 2\n1 2\n3 4\n" ||
					c.InputSHA256 != convolutionSHA256([]byte(c.Input)) || !slices.Equal(c.Expected, []uint32{3, 10, 8}) ||
					!slices.Equal(c.Actual, []uint32{4, 10, 8}) || r.Candidate.SHA256 != "candidate-id" {
					t.Fatalf("mismatch could not be replayed: %+v", r)
				}
			} else if r.Counterexample != nil || errors.Is(err, ErrConvolutionMismatch) {
				t.Fatal("non-mismatch produced a counterexample")
			}
			if tc.name == "matches" {
				raw, err := json.Marshal(r)
				if err != nil || bytes.Contains(raw, []byte(`"draft"`)) {
					t.Fatal("source-root report gained a draft field", err)
				}
			}
			want := []string{"verify oracle", "compile oracle", "remove oracle"}
			if !tc.oracleCleanup {
				want = append(want, "verify candidate", "compile candidate", "remove candidate")
			}
			if !slices.Equal(order, want) {
				t.Fatalf("programs or cleanup overlapped: %v", order)
			}
		})
	}
}

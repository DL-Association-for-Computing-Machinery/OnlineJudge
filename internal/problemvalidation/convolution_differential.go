package problemvalidation

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

// ErrConvolutionMismatch means both programs returned valid field elements
// for the same input and at least one element differed.
var ErrConvolutionMismatch = errors.New("candidate and oracle coefficients differ")

// ConvolutionCounterexample is private bounded evidence for replaying one case.
type ConvolutionCounterexample struct {
	Case        int      `json:"case"`
	Input       string   `json:"input"`
	InputSHA256 string   `json:"input_sha256"`
	Expected    []uint32 `json:"expected"`
	Actual      []uint32 `json:"actual"`
}

// ConvolutionDraftBinding identifies the exact private draft import used to
// prepare both solver inputs. It is provenance, not an authorization grant.
type ConvolutionDraftBinding struct {
	ID             string `json:"id"`
	Version        string `json:"version"`
	ImportRevision string `json:"import_revision"`
}

// ConvolutionReport describes a small differential trial, never publication
// approval. The Worker independently owns the slot and confirms its stop.
type ConvolutionReport struct {
	Schema         string                     `json:"schema"`
	Status         string                     `json:"status"`
	Publication    string                     `json:"publication"`
	SourceID       string                     `json:"source_id"`
	SourceRevision string                     `json:"source_revision"`
	SourceLock     string                     `json:"source_lock_sha256"`
	Draft          *ConvolutionDraftBinding   `json:"draft,omitempty"`
	Oracle         ConvolutionProgram         `json:"oracle"`
	Candidate      ConvolutionProgram         `json:"candidate"`
	Corpus         string                     `json:"corpus"`
	Seed           uint64                     `json:"seed"`
	CorpusSHA256   string                     `json:"corpus_sha256"`
	Cases          int                        `json:"cases"`
	OracleParsed   int                        `json:"oracle_parsed"`
	CandidateEqual int                        `json:"candidate_equal"`
	FailureRole    string                     `json:"failure_role,omitempty"`
	FailureStage   string                     `json:"failure_stage,omitempty"`
	FailureCase    int                        `json:"failure_case,omitempty"`
	Counterexample *ConvolutionCounterexample `json:"counterexample,omitempty"`
	ProfileSHA256  string                     `json:"profile_sha256"`
	ContainerID    string                     `json:"container_id,omitempty"`
	StartedAt      string                     `json:"started_at,omitempty"`
	StopConfirmed  bool                       `json:"stop_confirmed"`
	StopDiagnostic *StopDiagnostic            `json:"stop_diagnostic,omitempty"`
}

// NewConvolutionReport records the admitted identities even if startup never
// reached compilation. Only the slot owner may confirm a terminal outcome.
func NewConvolutionReport(sources ConvolutionSources) ConvolutionReport {
	report := ConvolutionReport{
		Schema: "oj.convolution-differential.v1", Status: "incomplete", Publication: "blocked",
		SourceID: ConvolutionSourceID, SourceRevision: sources.revision, SourceLock: SourceLockSHA256,
		Oracle: sources.oracle.identity, Candidate: sources.candidate.identity,
		Corpus: convolutionCorpusID, Seed: convolutionCorpusSeed, Cases: convolutionCases,
	}
	if binding := sources.draft; binding.ID != "" {
		report.Draft = &binding
	}
	return report
}

// RunSmallConvolution executes the frozen, pinned programs serially. The
// factory supplies a fresh adapter per program on the same exclusive host,
// whose own transport ledger spans both phases. The caller must stop the host
// even if admission, cleanup or cancellation makes the result incomplete.
func RunSmallConvolution(ctx context.Context, sources ConvolutionSources, factory func() (execution.Sandbox, error)) (ConvolutionReport, error) {
	report := NewConvolutionReport(sources)
	profile, _, err := ResolveSource(ConvolutionSourceID)
	if err != nil || ctx == nil || factory == nil || sources.oracle.source == "" || sources.candidate.source == "" {
		report.FailureStage = "admission"
		return report, errors.New("convolution differential admission failed")
	}
	report.ProfileSHA256 = profile.Digest()
	ctx, cancel := context.WithTimeout(ctx, profile.Wall)
	defer cancel()
	cases := smallConvolutionCorpus()
	report.CorpusSHA256 = convolutionCorpusDigest(cases)
	expected := make([][]uint32, len(cases))
	err = runConvolutionProgram(ctx, &report, profile.Runtime, "oracle", sources.oracle.source, cases, factory, func(i int, output []uint32) error {
		if !convolutionAnchor(i, output) {
			return errors.New("oracle did not match the hand-checked anchor")
		}
		expected[i] = output
		report.OracleParsed++
		return nil
	})
	if err != nil {
		return report, err
	}
	err = runConvolutionProgram(ctx, &report, profile.Runtime, "candidate", sources.candidate.source, cases, factory, func(i int, output []uint32) error {
		if !slices.Equal(expected[i], output) {
			input := cases[i].input
			report.Counterexample = &ConvolutionCounterexample{i + 1, string(input), convolutionSHA256(input), expected[i], output}
			return ErrConvolutionMismatch
		}
		report.CandidateEqual++
		return nil
	})
	if err == nil && ctx.Err() != nil {
		report.FailureRole, report.FailureStage = "candidate", "context"
		err = ctx.Err()
	}
	return report, err
}

func convolutionAnchor(i int, output []uint32) bool {
	switch i {
	case 9:
		return slices.Equal(output, []uint32{3, 10, 8})
	case 10:
		return slices.Equal(output, []uint32{1, 2, 1})
	case 11:
		return slices.Equal(output, []uint32{convolutionModulus - 1, 0, 1})
	default:
		return true
	}
}

func runConvolutionProgram(ctx context.Context, report *ConvolutionReport, profile experiment.ExecutionProfile, role, source string,
	cases []convolutionCase, factory func() (execution.Sandbox, error), accept func(int, []uint32) error) (err error) {
	stage, point := "adapter", 0
	defer func() {
		if err != nil {
			report.FailureRole, report.FailureStage, report.FailureCase = role, stage, point
		}
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	sandbox, err := factory()
	if err != nil || sandbox == nil {
		return errors.New("convolution adapter unavailable")
	}
	stage = "verify"
	verify, done := context.WithTimeout(ctx, 10*time.Second)
	err = sandbox.Verify(verify, profile)
	done()
	if err != nil {
		return err
	}
	stage = "compile"
	compile, done := context.WithTimeout(ctx, time.Duration(profile.Compile.Wall)+5*time.Second)
	compiled, err := sandbox.Compile(compile, profile, source)
	done()
	if compiled.Artifact != "" {
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if e := sandbox.Remove(cleanup, compiled.Artifact); e != nil {
				stage, point = "cleanup", 0
				err = errors.Join(err, e)
			}
		}()
	}
	if err != nil {
		return err
	}
	if verdict, e := execution.Classify(compiled, true); e != nil || verdict != "AC" || compiled.Artifact == "" {
		return errors.New("convolution compile evidence did not pass")
	}
	for i, c := range cases {
		stage, point = "run", i+1
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(c.input) == 0 || len(c.input) > convolutionCaseBytes {
			return errors.New("convolution input outside pilot budget")
		}
		run, done := context.WithTimeout(ctx, time.Duration(profile.Run.Wall)+5*time.Second)
		observation, err := sandbox.Run(run, profile, compiled.Artifact, c.input)
		done()
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if verdict, e := execution.Classify(observation, false); e != nil || verdict != "AC" || observation.OutputTruncated {
			return errors.New("convolution run evidence did not pass")
		}
		stage = "output"
		output, err := convolutionCoefficients(observation.Stdout, c.coefficients)
		if err != nil {
			return err
		}
		stage = "compare"
		if err := accept(i, output); err != nil {
			return err
		}
	}
	return ctx.Err()
}

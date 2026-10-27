// Package execution orchestrates bounded M0 attempts with optional lease supervision.
package execution

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
)

// ErrBusy rejects excess work instead of accumulating an unbounded local queue.
var ErrBusy = errors.New("execution slot occupied")

// Sandbox must verify actual environment identity and create a fresh isolation
// context per Run. Implementations must honor deadlines and bound all responses.
// A transport cancellation alone is not proof that a remote process has stopped.
type Sandbox interface {
	Verify(context.Context, experiment.ExecutionProfile) error
	Compile(context.Context, experiment.ExecutionProfile, string) (Observation, error)
	Run(context.Context, experiment.ExecutionProfile, string, []byte) (Observation, error)
	Remove(context.Context, string) error
}

// Observation preserves sandbox evidence; output and artifacts stay node-private.
type Observation struct {
	Status          string
	Exit            int
	CPU, Memory     int64
	Stdout          []byte
	Artifact        string
	OutputTruncated bool
}

// Point records only executed tests. Missing points remain explicitly NOT_RUN.
type Point struct {
	ID, Verdict     string
	Usage           *Usage
	OutputTruncated bool
}

// Usage is measured CPU nanoseconds and process-group peak memory bytes.
type Usage struct{ CPU, Memory int64 }

// Result retains compilation separately; maxima include only executed test points.
// It is internal evidence, not a lease report or a public response model.
type Result struct {
	Verdict string
	Compile *Observation
	Points  []Point
	Max     *Usage
}

// Runner owns one execution slot. Use one instance per sandbox of capacity one.
type Runner struct {
	sandbox   Sandbox
	busy      atomic.Bool
	unhealthy atomic.Bool
}

// New binds the node sandbox. No execution happens until Judge is called.
func New(s Sandbox) *Runner { return &Runner{sandbox: s} }

// Judge verifies all materials before compilation, compiles once and stops at
// the first failed point. Any infrastructure/cleanup error yields SE and an error.
// Lease renewal, remote kill acknowledgement and reporting are separate work.
func (r *Runner) Judge(ctx context.Context, d materials.Descriptor, blobs map[string][]byte) (result Result, err error) {
	result.Verdict = "SE"
	if !r.busy.CompareAndSwap(false, true) {
		return result, ErrBusy
	}
	defer r.busy.Store(false)
	if r.unhealthy.Load() {
		return result, errors.New("sandbox requires recovery")
	}
	p, err := prepare(d, blobs)
	if err != nil {
		return result, err
	}
	for _, t := range p.Tests {
		result.Points = append(result.Points, Point{ID: t.ID, Verdict: "NOT_RUN"})
	}
	if r.sandbox == nil {
		return result, errors.New("sandbox unavailable")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	verifyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = r.sandbox.Verify(verifyCtx, p.Environment)
	cancel()
	if err != nil {
		return result, err
	}
	// Failed or cancelled remote execution has an unknown completion state. Do
	// not reuse its capacity until the owner recovers the sandbox and new runner.
	defer func() {
		if err != nil {
			r.unhealthy.Store(true)
		}
	}()
	compileCtx, cancel := context.WithTimeout(ctx, time.Duration(p.Environment.Compile.Wall)+5*time.Second)
	compiled, err := r.sandbox.Compile(compileCtx, p.Environment, d.Source)
	cancel()
	// An artifact can accompany a failed response; clean it on every exit path.
	if compiled.Artifact != "" {
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if cleanupErr := r.sandbox.Remove(cleanupCtx, compiled.Artifact); cleanupErr != nil {
				result.Verdict = "SE"
				err = errors.Join(err, cleanupErr)
			}
		}()
	}
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	result.Compile = &compiled
	verdict, err := classify(compiled, true)
	if err != nil {
		return result, err
	}
	if verdict != "AC" {
		result.Verdict = verdict
		return result, nil
	}
	if compiled.Artifact == "" {
		return result, errors.New("compiler returned no artifact")
	}
	for i, t := range p.Tests {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		runCtx, cancel := context.WithTimeout(ctx, time.Duration(p.Environment.Run.Wall)+5*time.Second)
		observed, runErr := r.sandbox.Run(runCtx, p.Environment, compiled.Artifact, blobs[t.Input])
		cancel()
		result.Points[i].Verdict = "SE"
		if runErr != nil {
			return result, runErr
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		verdict, err := classify(observed, false)
		if err != nil {
			return result, err
		}
		result.Points[i].Usage = &Usage{CPU: observed.CPU, Memory: observed.Memory}
		result.Points[i].OutputTruncated = observed.OutputTruncated
		if result.Max == nil {
			result.Max = &Usage{}
		}
		result.Max.CPU = max(result.Max.CPU, observed.CPU)
		result.Max.Memory = max(result.Max.Memory, observed.Memory)
		if verdict == "AC" {
			ok, compareErr := judging.CompareText(blobs[t.Answer], observed.Stdout, p.CaseSensitive)
			if compareErr != nil {
				return result, compareErr
			}
			if !ok {
				verdict = "WA"
			}
		}
		result.Points[i].Verdict = verdict
		if verdict != "AC" {
			result.Verdict = verdict
			return result, nil
		}
	}
	result.Verdict = "AC"
	return result, nil
}

func classify(o Observation, compile bool) (string, error) {
	if o.CPU < 0 || o.Memory < 0 {
		return "SE", errors.New("invalid resource evidence")
	}
	if o.Status == "Accepted" && o.Exit == 0 {
		return "AC", nil
	}
	if compile {
		switch o.Status {
		case "Nonzero Exit Status":
			if o.Exit != 0 {
				return "CE", nil
			}
		case "Time Limit Exceeded", "Memory Limit Exceeded", "Output Limit Exceeded":
			return "CE", nil
		}
	} else {
		switch o.Status {
		case "Time Limit Exceeded":
			return "TLE", nil
		case "Memory Limit Exceeded":
			return "MLE", nil
		case "Output Limit Exceeded":
			return "OLE", nil
		case "Nonzero Exit Status", "Signalled", "Dangerous Syscall":
			if o.Exit != 0 {
				return "RE", nil
			}
		}
	}
	return "SE", errors.New("untrusted sandbox outcome")
}

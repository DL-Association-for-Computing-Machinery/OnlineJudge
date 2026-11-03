package problemvalidation

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

// Point contains execution evidence only, never stdout or reference bytes.
type Point struct {
	ID      string           `json:"id"`
	Verdict string           `json:"verdict"`
	Usage   *execution.Usage `json:"usage,omitempty"`
}

// StopDiagnostic contains only the host adapter's bounded failure projection.
// It supplies a diagnosis, never authority to accept a failed reference report.
type StopDiagnostic struct {
	Stage     string `json:"stage"`
	Code      string `json:"code"`
	Observed  int    `json:"observed"`
	Confirmed int    `json:"confirmed"`
	Probe     int    `json:"probe,omitempty"`
}

// Report describes reference correctness only. A pass requires a separate owner
// to confirm sandbox stop, marker cleanup and report persistence; it is never a
// complete problem-validation or publication authorization.
type Report struct {
	Schema         string                `json:"schema"`
	Status         string                `json:"status"`
	Publication    string                `json:"publication"`
	Profile        Profile               `json:"profile"`
	ProfileSHA256  string                `json:"profile_sha256"`
	PlanSHA256     string                `json:"plan_sha256,omitempty"`
	Plan           problem.ReferencePlan `json:"plan"`
	Verdict        string                `json:"verdict"`
	Points         []Point               `json:"points"`
	CompileUsage   *execution.Usage      `json:"compile_usage,omitempty"`
	StopConfirmed  bool                  `json:"stop_confirmed"`
	StopDiagnostic *StopDiagnostic       `json:"stop_diagnostic,omitempty"`
	Slot           string                `json:"slot"`
	ContainerID    string                `json:"container_id"`
	StartedAt      string                `json:"started_at"`
}

// Runner owns one operation. The owner must stop its sandbox even after success;
// an adapter is never reused after an error or a second Validate call.
type Runner struct {
	sandbox execution.Sandbox
	used    atomic.Bool
}

// New binds a verified-capable sandbox without starting work.
func New(s execution.Sandbox) *Runner { return &Runner{sandbox: s} }

// Validate freezes and prechecks every reference before compilation, compiles
// once and runs fresh point environments in manifest order until first failure.
// Artifact cleanup uses an independent deadline; HTTP cancellation alone never
// proves sandbox stop. Finalize must be called after the owner confirms stop.
func (r *Runner) Validate(ctx context.Context, prepared *problem.Prepared, profile Profile) (report Report, err error) {
	return r.validate(ctx, prepared, profile, "library-checker:aplusb")
}

// ValidateUnionFind admits the independently pinned disk snapshot/runtime. All
// answers are prechecked before the first execution adapter call.
func (r *Runner) ValidateUnionFind(ctx context.Context, prepared *problem.Prepared, profile Profile) (report Report, err error) {
	return r.validate(ctx, prepared, profile, "library-checker:unionfind")
}

// ValidateSource selects only a complete admitted source's fixed profile. It
// cannot run one Prepared under another source's resource admission.
func (r *Runner) ValidateSource(ctx context.Context, prepared *problem.Prepared, sourceID string) (Report, error) {
	p, _, err := ResolveSource(sourceID)
	if err != nil {
		return Report{}, err
	}
	return r.validate(ctx, prepared, p, sourceID)
}

func (r *Runner) validate(ctx context.Context, prepared *problem.Prepared, profile Profile, sourceID string) (report Report, err error) {
	report = Report{Schema: "oj.reference-validation.v1", Status: "reference-validation-failed", Publication: "blocked", Profile: profile, ProfileSHA256: profile.Digest(), Verdict: "SE", Points: []Point{}}
	admitted, _, admissionErr := ResolveSource(sourceID)
	if r == nil || ctx == nil || r.sandbox == nil || admissionErr != nil || !r.used.CompareAndSwap(false, true) || !reflect.DeepEqual(profile, admitted) {
		return report, errors.New("unsupported or occupied reference validation")
	}
	if prepared.Content().SourceID != sourceID {
		return report, errors.New("reference source identity mismatch")
	}
	ctx, cancel := context.WithTimeout(ctx, profile.Wall)
	defer cancel()
	defer func() {
		if e := ctx.Err(); e != nil {
			report.Verdict = "SE"
			err = errors.Join(err, e)
		}
	}()
	plan, frozen, err := freezeReferences(ctx, prepared, profile, sourceID != "library-checker:aplusb")
	report.Plan = plan
	for _, c := range plan.Cases {
		report.Points = append(report.Points, Point{ID: c.ID, Verdict: "NOT_RUN"})
	}
	if err != nil {
		return report, err
	}
	defer func() {
		if e := frozen.Close(); e != nil {
			report.Verdict = "SE"
			err = errors.Join(err, e)
		}
	}()
	if plan.SourceID != sourceID {
		return report, errors.New("frozen reference source identity mismatch")
	}
	report.Plan, report.PlanSHA256 = plan, plan.Digest()
	for i := range plan.Cases {
		answer, err := frozen.ReadAnswer(ctx, i)
		if err != nil {
			return report, err
		}
		if _, err := judging.CompareLibraryCheckerText(answer, nil, profile.Text); err != nil {
			return report, err
		}
	}
	source, err := frozen.ReadSource(ctx)
	if err != nil {
		return report, err
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	verifyCtx, done := context.WithTimeout(ctx, 10*time.Second)
	err = r.sandbox.Verify(verifyCtx, profile.Runtime)
	done()
	if err != nil {
		return report, err
	}
	compileCtx, done := context.WithTimeout(ctx, time.Duration(profile.Runtime.Compile.Wall)+5*time.Second)
	compiled, err := r.sandbox.Compile(compileCtx, profile.Runtime, string(source))
	done()
	if compiled.Artifact != "" {
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if e := r.sandbox.Remove(cleanup, compiled.Artifact); e != nil {
				report.Verdict = "SE"
				err = errors.Join(err, e)
			}
		}()
	}
	if err != nil {
		return report, err
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	verdict, err := execution.Classify(compiled, true)
	if err != nil {
		return report, err
	}
	report.CompileUsage = &execution.Usage{CPU: compiled.CPU, Memory: compiled.Memory}
	if verdict != "AC" {
		report.Verdict = verdict
		return report, nil
	}
	if compiled.Artifact == "" {
		return report, errors.New("reference compiler returned no artifact")
	}
	for i, c := range plan.Cases {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		input, answer, err := frozen.ReadCase(ctx, i)
		if err != nil {
			return report, err
		}
		runCtx, done := context.WithTimeout(ctx, time.Duration(profile.Runtime.Run.Wall)+5*time.Second)
		o, runErr := r.sandbox.Run(runCtx, profile.Runtime, compiled.Artifact, input)
		done()
		report.Points[i].Verdict = "SE"
		if runErr != nil {
			return report, runErr
		}
		if err := ctx.Err(); err != nil {
			return report, err
		}
		verdict, e := execution.Classify(o, false)
		if e != nil {
			return report, e
		}
		if verdict == "AC" {
			ok, e := judging.CompareLibraryCheckerText(answer, o.Stdout, profile.Text)
			if e != nil {
				return report, e
			}
			if !ok {
				verdict = "WA"
			}
		}
		report.Points[i] = Point{ID: c.ID, Verdict: verdict, Usage: &execution.Usage{CPU: o.CPU, Memory: o.Memory}}
		if verdict != "AC" {
			report.Verdict = verdict
			return report, nil
		}
	}
	report.Verdict = "AC"
	return report, nil
}

// Finalize fences a pass behind actual stop and marker-clean confirmation. The
// owner must also persist the resulting report before announcing completion.
func (r *Report) Finalize(executionErr, stopErr, cleanErr error) error {
	r.StopConfirmed = stopErr == nil
	err := errors.Join(executionErr, stopErr, cleanErr)
	if err != nil {
		r.Status = "reference-validation-failed"
		r.Verdict = "SE"
		return err
	}
	if r.Verdict == "AC" {
		r.Status = "reference-validation-passed"
	}
	return nil
}

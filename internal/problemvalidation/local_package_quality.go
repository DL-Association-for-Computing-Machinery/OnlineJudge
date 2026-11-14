package problemvalidation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

const PackageQualitySchema = "oj.local-text-package-quality.v1"

// PackageQualityConfig selects only files pinned by the bundle source lock.
// Witnesses use one-based formal positions and must refer to non-sample points.
type PackageQualityConfig struct {
	Controls []PackageInputControl `json:"controls"`
	Probes   []string              `json:"probes"`
	Coverage map[string][]int      `json:"coverage"`
	Wrong    []string              `json:"wrong"`
}

type PackageInputControl struct {
	Input  string `json:"input"`
	Accept bool   `json:"accept"`
}

// PackageQualityReport is emitted only by the fixed owned sandbox workflow.
// It carries result and material identities, never private inputs or programs.
type PackageQualityReport struct {
	Schema         string  `json:"schema"`
	ImportRevision string  `json:"import_revision"`
	Plan           string  `json:"plan_sha256"`
	Profile        string  `json:"profile_sha256"`
	Config         string  `json:"config_sha256"`
	Inputs         int     `json:"inputs"`
	Controls       int     `json:"controls"`
	OracleCases    int     `json:"oracle_cases"`
	SamplePoints   int     `json:"sample_points"`
	Criteria       int     `json:"non_sample_criteria"`
	Wrong          [][]int `json:"wrong_rejections"`
	StopConfirmed  bool    `json:"stop_confirmed"`
}

type packageQuality struct {
	config  PackageQualityConfig
	hash    string
	samples []bool
}

func readPackageQuality(ctx context.Context, p *problem.Prepared) (packageQuality, error) {
	var q packageQuality
	raw, err := p.ReadLocalTextQualityFile(ctx, "quality/plan.json", 16<<10)
	if err != nil {
		return q, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&q.config) != nil || dec.Decode(new(any)) != io.EOF {
		return q, ErrCheckerTrialEvidence
	}
	canonical, err := json.Marshal(q.config)
	if err != nil || !bytes.Equal(raw, append(canonical, '\n')) {
		return q, ErrCheckerTrialEvidence
	}
	_, q.samples, err = p.LocalTextDisplayBinding()
	q.hash = handoffDigest(raw)
	if err != nil || len(q.config.Controls) != 4 || len(q.config.Probes) < 10 || len(q.config.Probes) > 32 ||
		len(q.config.Coverage) == 0 || len(q.config.Coverage) > 32 || len(q.config.Wrong) != 4 {
		return q, ErrCheckerTrialEvidence
	}
	isName := func(s string) bool {
		return len(s) > 0 && len(s) <= 48 && strings.Trim(s, "abcdefghijklmnopqrstuvwxyz0123456789-_") == ""
	}
	names := make(map[string]bool)
	for _, name := range append(slices.Clone(q.config.Probes), q.config.Wrong...) {
		if !isName(name) || names[name] {
			return q, ErrCheckerTrialEvidence
		}
		names[name] = true
	}
	accepted := 0
	for _, control := range q.config.Controls {
		if !isName(control.Input) || names[control.Input] {
			return q, ErrCheckerTrialEvidence
		}
		names[control.Input] = true
		if control.Accept {
			accepted++
		}
	}
	for label, witnesses := range q.config.Coverage {
		if !isName(label) || len(witnesses) == 0 || len(witnesses) > len(q.samples) {
			return q, ErrCheckerTrialEvidence
		}
		for i, point := range witnesses {
			if !q.nonSample(point) || i > 0 && point <= witnesses[i-1] {
				return q, ErrCheckerTrialEvidence
			}
		}
	}
	if accepted != 1 {
		return q, ErrCheckerTrialEvidence
	}
	return q, nil
}

func (q packageQuality) nonSample(n int) bool {
	return n > 0 && n <= len(q.samples) && !q.samples[n-1]
}

func (q packageQuality) report(plan problem.ReferencePlan) PackageQualityReport {
	r := PackageQualityReport{Schema: PackageQualitySchema, ImportRevision: plan.ImportRevision, Plan: plan.Digest(),
		Profile: LocalTextProfile().Digest(), Config: q.hash, Inputs: len(plan.Cases), Controls: len(q.config.Controls),
		OracleCases: len(q.config.Probes), Criteria: len(q.config.Coverage)}
	for _, sample := range q.samples {
		if sample {
			r.SamplePoints++
		}
	}
	return r
}

// RunPackageQuality runs the pinned input validator, independent oracle and
// wrong candidates in the already owned fixed guest. Its caller confirms stop.
func RunPackageQuality(ctx context.Context, p *problem.Prepared, judge func() (*execution.CachedGoJudge, error)) (_ PackageQualityReport, err error) {
	q, err := readPackageQuality(ctx, p)
	if err != nil {
		return PackageQualityReport{}, err
	}
	frozen, err := p.PrepareLocalTextReviewReference(ctx, ctx)
	if err != nil {
		return PackageQualityReport{}, err
	}
	defer func() { err = errors.Join(err, frozen.Close()) }()
	plan := frozen.Plan()
	r := q.report(plan)
	if plan.ValidateMaterials() != nil || len(q.samples) != r.Inputs {
		return r, ErrCheckerTrialEvidence
	}
	read := func(name string) ([]byte, error) { return p.ReadLocalTextQualityFile(ctx, "quality/"+name, 64<<10) }
	runProgram := func(src []byte, work func(func([]byte) (execution.Observation, error)) error) (err error) {
		g, err := judge()
		profile := LocalTextProfile().Runtime
		if err != nil {
			return err
		}
		if err := g.Verify(ctx, profile); err != nil {
			return err
		}
		o, err := g.Compile(ctx, profile, string(src))
		if o.Artifact != "" {
			defer func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				err = errors.Join(err, g.Remove(cleanup, o.Artifact))
			}()
		}
		if err != nil || o.Status != "Accepted" || o.Exit != 0 || o.Artifact == "" {
			return errors.Join(ErrCheckerTrialEvidence, err)
		}
		return work(func(in []byte) (execution.Observation, error) {
			return g.RunCheckerTrial(ctx, profile, o.Artifact, in)
		})
	}
	err = q.run(ctx, frozen, read, runProgram, &r)
	return r, err
}

type qualityProgram func([]byte, func(func([]byte) (execution.Observation, error)) error) error

func (q packageQuality) run(ctx context.Context, frozen *problem.ReferenceSnapshot, read func(string) ([]byte, error), runProgram qualityProgram, report *PackageQualityReport) error {
	verify, err := read("validator.cpp")
	if err != nil {
		return err
	}
	if err := runProgram(verify, func(run func([]byte) (execution.Observation, error)) error {
		check := func(in []byte, native error, accept bool) error {
			if native != nil {
				return native
			}
			o, err := run(in)
			if err != nil {
				return err
			}
			if accept && o.Status == "Accepted" && o.Exit == 0 || !accept && o.Status == "Nonzero Exit Status" && o.Exit == 1 {
				return nil
			}
			return ErrCheckerTrialEvidence
		}
		for i := range q.samples {
			raw, err := frozen.ReadInput(ctx, i)
			if err := check(raw, err, true); err != nil {
				return fmt.Errorf("validator formal point %d: %w", i+1, err)
			}
		}
		seen := make(map[string]bool)
		for _, c := range q.config.Controls {
			raw, err := read("inputs/" + c.Input + ".in")
			key := handoffDigest(raw)
			if err != nil || seen[key] {
				return ErrCheckerTrialEvidence
			}
			seen[key] = true
			if err := check(raw, err, c.Accept); err != nil {
				return fmt.Errorf("validator control: %w", err)
			}
		}
		for _, name := range q.config.Probes {
			raw, err := read("inputs/" + name + ".in")
			if err := check(raw, err, true); err != nil {
				return fmt.Errorf("oracle input control: %w", err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	source, err := frozen.ReadSource(ctx)
	if err != nil {
		return err
	}
	oracle, err := read("oracle.cpp")
	if err != nil || bytes.Equal(oracle, source) || bytes.Equal(oracle, verify) {
		return ErrCheckerTrialEvidence
	}
	var probes [][]byte
	seenProbe := map[string]bool{}
	for _, name := range q.config.Probes {
		raw, err := read("inputs/" + name + ".in")
		if err != nil || seenProbe[handoffDigest(raw)] {
			return ErrCheckerTrialEvidence
		}
		seenProbe[handoffDigest(raw)] = true
		probes = append(probes, raw)
	}
	compare := func(a, b []byte) (bool, error) {
		return judging.CompareLibraryCheckerText(a, b, localTextLimits)
	}
	accepted := func(run func([]byte) (execution.Observation, error), raw []byte) ([]byte, error) {
		o, err := run(raw)
		if err != nil || o.Status != "Accepted" || o.Exit != 0 {
			return nil, errors.Join(ErrCheckerTrialEvidence, err)
		}
		return o.Stdout, nil
	}
	var expected [][]byte
	if err := runProgram(oracle, func(run func([]byte) (execution.Observation, error)) error {
		for _, raw := range probes {
			out, err := accepted(run, raw)
			if err != nil {
				return err
			}
			expected = append(expected, out)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := runProgram(source, func(run func([]byte) (execution.Observation, error)) error {
		for i, raw := range probes {
			out, err := accepted(run, raw)
			if err != nil {
				return err
			}
			same, err := compare(expected[i], out)
			if err != nil || !same {
				return ErrCheckerTrialEvidence
			}
		}
		return nil
	}); err != nil {
		return err
	}
	programs := map[string]bool{handoffDigest(source): true, handoffDigest(oracle): true, handoffDigest(verify): true}
	coverage, err := read("coverage.cpp")
	if err != nil || programs[handoffDigest(coverage)] {
		return ErrCheckerTrialEvidence
	}
	programs[handoffDigest(coverage)] = true
	if err := runProgram(coverage, func(run func([]byte) (execution.Observation, error)) error {
		absent := make(map[string]bool)
		for i := range q.samples {
			if !q.nonSample(i + 1) {
				continue
			}
			in, err := frozen.ReadInput(ctx, i)
			if err != nil {
				return err
			}
			out, err := accepted(run, in)
			if err != nil {
				return err
			}
			labels := strings.Fields(string(out))
			for name, points := range q.config.Coverage {
				found := slices.Contains(labels, name)
				if slices.Contains(points, i+1) && !found {
					return ErrCheckerTrialEvidence
				}
				absent[name] = absent[name] || !found
			}
		}
		// A predicate which simply prints every label cannot establish coverage.
		for name := range q.config.Coverage {
			if !absent[name] {
				return ErrCheckerTrialEvidence
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("coverage: %w", err)
	}
	for _, name := range q.config.Wrong {
		src, err := read("wrong/" + name + ".cpp")
		if err != nil || programs[handoffDigest(src)] {
			return ErrCheckerTrialEvidence
		}
		programs[handoffDigest(src)] = true
		var rejected []int
		if err := runProgram(src, func(run func([]byte) (execution.Observation, error)) error {
			for i := range q.samples {
				if !q.nonSample(i + 1) {
					continue
				}
				in, ans, err := frozen.ReadCase(ctx, i)
				if err != nil {
					return err
				}
				out, err := accepted(run, in)
				if err != nil {
					return err
				}
				same, err := compare(ans, out)
				if err != nil {
					return err
				}
				if !same {
					rejected = append(rejected, i+1)
				}
			}
			if len(rejected) == 0 {
				return ErrCheckerTrialEvidence
			}
			return nil
		}); err != nil {
			return fmt.Errorf("wrong candidate %s: %w", name, err)
		}
		for _, prev := range report.Wrong {
			if slices.Equal(prev, rejected) {
				return ErrCheckerTrialEvidence
			}
		}
		report.Wrong = append(report.Wrong, rejected)
	}
	return ctx.Err()
}

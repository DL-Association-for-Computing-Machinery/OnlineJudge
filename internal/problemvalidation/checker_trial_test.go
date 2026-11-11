package problemvalidation_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestCheckerTrialBindsCandidateAndRejectsOtherRuntime(t *testing.T) {
	_, p := fixture(t)
	ctx := t.Context()
	plan, err := p.PrepareReference(ctx, problem.SmallReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	limits := judging.TextLimits{Bytes: experiment.PinnedReferenceEnvironment().OutputBytes, TokenBytes: 12}
	c, err := problemvalidation.PreflightCheckerCandidate(ctx, p, plan.Digest(), judging.LibraryCheckerTextVersion, limits)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		plan, candidate string
		limits          judging.TextLimits
	}{
		{plan.Digest(), "", limits},
		{plan.Digest(), strings.Repeat("1", 64), limits},
		{strings.Repeat("1", 64), c.Digest(), limits},
		{plan.Digest(), c.Digest(), judging.TextLimits{Bytes: limits.Bytes - 1, TokenBytes: 12}},
		{plan.Digest(), c.Digest(), judging.TextLimits{Bytes: limits.Bytes + 1, TokenBytes: 12}},
		{plan.Digest(), c.Digest(), judging.TextLimits{Bytes: limits.Bytes, TokenBytes: 13}},
	} {
		trial, err := problemvalidation.PrepareCheckerTrial(ctx, p, tc.plan, tc.candidate, tc.limits)
		if trial != nil || !errors.Is(err, problemvalidation.ErrCheckerCandidateBinding) {
			t.Fatal("invalid binding prepared a trial", err)
		}
	}
	trial, err := problemvalidation.PrepareCheckerTrial(ctx, p, plan.Digest(), c.Digest(), limits)
	if err != nil {
		t.Fatal(err)
	}
	g := execution.NewCachedGoJudge(nil, nil, nil)
	report, err := trial.Run(ctx, g)
	if err == nil || report.Complete || report.Verdict != "SE" || report.CandidateSHA256 != c.Digest() ||
		report.RuntimeSHA256 != report.RuntimeProfile.Digest() || len(report.Points) != 2 ||
		report.Points[0].Verdict != "NOT_RUN" || report.Points[1].Verdict != "NOT_RUN" {
		t.Fatalf("wrong runtime produced evidence: %+v, %v", report, err)
	}
	if _, err := trial.Run(ctx, g); err == nil {
		t.Fatal("used trial accepted another run")
	}
}

func TestCheckerTrialRequiresSuccessfulStopAndCleanup(t *testing.T) {
	failed := errors.New("unconfirmed")
	for _, tc := range []struct {
		run, stop, clean error
	}{
		{run: failed}, {stop: failed}, {clean: failed},
	} {
		report := problemvalidation.CheckerTrialReport{Schema: "oj.checker-trial.v1", Verdict: "AC"}
		if err := report.Finalize(tc.run, tc.stop, tc.clean); !errors.Is(err, failed) ||
			report.Complete || report.Verdict != "SE" || report.StopConfirmed != (tc.stop == nil) {
			t.Fatalf("failed evidence finalized: %+v, %v", report, err)
		}
	}
	report := problemvalidation.CheckerTrialReport{Schema: "oj.checker-trial.v1", Verdict: "WA"}
	if err := report.Finalize(nil, nil, nil); err != nil || !report.Complete || !report.StopConfirmed || report.Verdict != "WA" {
		t.Fatalf("conclusive mismatch was lost: %+v, %v", report, err)
	}
}

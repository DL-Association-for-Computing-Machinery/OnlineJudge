package problemvalidation_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestDiskCheckerTrialOwnsFrozenSnapshot(t *testing.T) {
	for _, tc := range []struct {
		source, original, profile string
		bundle                    func(*testing.T) *problemfixture.Bundle
		tier                      experiment.ReferenceTier
		bytes                     int
	}{
		{"unionfind", "data_structure/unionfind/sol/correct.cpp", "p4-checker-candidate-unionfind-reference-v1", problemfixture.NewUnionFind, experiment.ReferenceSmall, 256 << 10},
		{"zalgorithm", "string/zalgorithm/sol/correct.cpp", "p4-checker-candidate-zalgorithm-reference-v1", problemfixture.NewZAlgorithm, experiment.ReferenceMedium, 4 << 20},
	} {
		t.Run(tc.source, func(t *testing.T) {
			testDiskCheckerTrial(t, tc.bundle(t), tc.source, tc.original, tc.profile, tc.tier, tc.bytes)
		})
	}
}

func testDiskCheckerTrial(t *testing.T, b *problemfixture.Bundle, source, original, profile string, tier experiment.ReferenceTier, outputBytes int) {
	t.Helper()
	b.AddOriginal(t, original, []byte("int main(){}"))
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, "library-checker:"+source, b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	snapshot, err := p.PrepareDiskReference(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	plan := snapshot.Plan().Digest()
	if err := snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	limits := judging.TextLimits{Bytes: outputBytes, TokenBytes: 4096}
	c, err := problemvalidation.PreflightCheckerCandidate(t.Context(), p, plan, judging.LibraryCheckerTextVersion, limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := problemvalidation.PrepareCheckerTrial(t.Context(), p, plan, strings.Repeat("1", 64), limits); !errors.Is(err, problemvalidation.ErrCheckerCandidateBinding) {
		t.Fatalf("unexpected wrong candidate result: %v", err)
	}
	wrong := judging.TextLimits{Bytes: outputBytes / 2, TokenBytes: 4096}
	wrongCandidate, err := problemvalidation.PreflightCheckerCandidate(t.Context(), p, plan, judging.LibraryCheckerTextVersion, wrong)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := problemvalidation.PrepareCheckerTrial(t.Context(), p, plan, wrongCandidate.Digest(), wrong); !errors.Is(err, problemvalidation.ErrCheckerCandidateBinding) {
		t.Fatalf("candidate with nonmatching runtime cap was not rejected: %v", err)
	}
	prepare, cancel := context.WithCancel(t.Context())
	defer cancel()
	trial, err := problemvalidation.PrepareCheckerTrialWithLifetime(prepare, t.Context(), p, plan, c.Digest(), limits)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = trial.Close() })
	cancel()
	if !errors.Is(p.Close(), problem.ErrBusy) {
		t.Fatal("trial released its materials before execution")
	}
	// Reaching the runtime verification diagnostic proves the source can still
	// be read after the preflight context has been canceled.
	report, err := trial.Run(t.Context(), execution.NewCachedGoJudge(nil, nil, nil))
	if err == nil || report.Diagnostic == nil || trial.Tier() != tier || report.RuntimeProfile.ID != profile ||
		report.FormalPlanSHA != plan || report.CandidateSHA256 != c.Digest() {
		t.Fatalf("trial did not reach the pinned runtime check: %+v, %v", report, err)
	}
	if err := errors.Join(trial.Close(), p.Close()); err != nil {
		t.Fatal(err)
	}
}

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

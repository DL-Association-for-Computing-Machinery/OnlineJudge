package problemvalidation_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestCheckerCandidateBindsExplicitLimitsAndFrozenPlan(t *testing.T) {
	b, p := fixture(t)
	plan, err := p.PrepareReference(t.Context(), problem.SmallReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	id, err := p.ImportIdentity()
	if err != nil {
		t.Fatal(err)
	}
	pin := plan.Digest()
	version := judging.LibraryCheckerTextVersion
	build := func(ctx context.Context, expected, version string, caps judging.TextLimits) (problemvalidation.CheckerCandidate, error) {
		return problemvalidation.PreflightCheckerCandidate(ctx, p, expected, version, caps)
	}
	caps := judging.TextLimits{Bytes: 2, TokenBytes: 1}
	a, err := build(t.Context(), pin, version, caps)
	if err != nil || a.FormalPlanSHA256 != pin || a.ContentVersion != id.Version || a.ImportRevision != id.ImportRevision ||
		a.SourceLockSHA256 != id.SourceLockSHA256 || a.SourceID != id.SourceID || a.AnswerCount != 2 ||
		a.Bytes != 2 || a.TokenBytes != 1 || a.CheckerVersion != version || a.Schema != problemvalidation.CheckerCandidateSchema {
		t.Fatalf("candidate lost checked bindings: %+v, %v", a, err)
	}
	if again, err := build(t.Context(), pin, version, caps); err != nil || again != a || again.Digest() != a.Digest() {
		t.Fatal("same frozen inputs produced a different candidate", err)
	}
	for _, changed := range []judging.TextLimits{{Bytes: 3, TokenBytes: 1}, {Bytes: 2, TokenBytes: 2}} {
		other, err := build(t.Context(), pin, version, changed)
		if err != nil || other.Digest() == a.Digest() {
			t.Fatal("different explicit limit reused candidate identity", err)
		}
	}
	plan.Cases[0], plan.Cases[1] = plan.Cases[1], plan.Cases[0]
	reordered := plan.Digest()
	plan.Cases[0], plan.Cases[1] = plan.Cases[1], plan.Cases[0]
	secondAnswer := plan.Cases[1].Answer.SHA256
	plan.Cases[1].Answer.SHA256 = strings.Repeat("e", 64)
	changedAnswer := plan.Digest()
	plan.Cases[1].Answer.SHA256 = secondAnswer
	plan.ImportRevision = strings.Repeat("d", 64)
	changedRevision := plan.Digest()
	for _, wrong := range []string{"", reordered, changedAnswer, changedRevision} {
		if got, err := build(t.Context(), wrong, version, caps); got != (problemvalidation.CheckerCandidate{}) || !errors.Is(err, problemvalidation.ErrCheckerCandidateBinding) {
			t.Fatalf("wrong trusted plan accepted: %+v, %v", got, err)
		}
	}
	for _, invalid := range []judging.TextLimits{{}, {Bytes: judging.MaxTextBytes + 1, TokenBytes: 1}, {Bytes: 2, TokenBytes: 3}} {
		if got, err := build(t.Context(), pin, version, invalid); got != (problemvalidation.CheckerCandidate{}) || !errors.Is(err, judging.ErrTextLimits) {
			t.Fatalf("invalid caps accepted: %+v, %v", got, err)
		}
	}
	if got, err := build(t.Context(), pin, "unknown", caps); got != (problemvalidation.CheckerCandidate{}) || err == nil {
		t.Fatalf("unknown version accepted: %+v, %v", got, err)
	}
	if got, err := build(t.Context(), pin, version, judging.TextLimits{Bytes: 1, TokenBytes: 1}); got != (problemvalidation.CheckerCandidate{}) || !errors.Is(err, judging.ErrReference) {
		t.Fatalf("too-small cap accepted: %+v, %v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := build(ctx, pin, version, caps); got != (problemvalidation.CheckerCandidate{}) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation produced a candidate: %+v, %v", got, err)
	}
	path := filepath.Join(b.Directory, "objects", "sha256", plan.Cases[1].Answer.SHA256)
	if err := os.WriteFile(path, []byte("6\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := build(t.Context(), pin, version, caps); got != (problemvalidation.CheckerCandidate{}) || err == nil {
		t.Fatalf("same-size answer substitution accepted: %+v, %v", got, err)
	}
}

func TestCheckerCandidateChecksLastDiskAnswerAndReleasesSnapshot(t *testing.T) {
	_, p := unionRunnerFixture(t, []byte("1234\n"))
	s, err := p.PrepareDiskReference(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	plan := s.Plan()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	build := func(caps judging.TextLimits) (problemvalidation.CheckerCandidate, error) {
		return problemvalidation.PreflightCheckerCandidate(t.Context(), p, plan.Digest(), judging.LibraryCheckerTextVersion, caps)
	}
	if c, err := build(judging.TextLimits{Bytes: 5, TokenBytes: 3}); c != (problemvalidation.CheckerCandidate{}) ||
		!errors.Is(err, judging.ErrReference) || !strings.Contains(err.Error(), plan.Cases[len(plan.Cases)-1].ID) {
		t.Fatalf("last invalid token did not block the candidate: %+v, %v", c, err)
	}
	if c, err := build(judging.TextLimits{Bytes: 5, TokenBytes: 4}); err != nil || c.AnswerCount != len(plan.Cases) || c.FormalPlanSHA256 != plan.Digest() {
		t.Fatalf("corrected cap could not reuse released snapshot slot: %+v, %v", c, err)
	}
}

package problemvalidation_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestAPlusBAMD64CandidateCannotPrepareExecution(t *testing.T) {
	const source = "int main() { return 0; }"
	h := sha256.Sum256([]byte(source))
	trial, err := problemvalidation.PrepareAPlusBJudgeForTier(t.Context(), nil,
		problemvalidation.ReviewedAPlusBPlan, problemvalidation.ReviewedAPlusBCandidate,
		strings.Repeat("a", 64), hex.EncodeToString(h[:]), source, problemvalidation.ReviewedAPlusBLimits(),
		experiment.ReferenceAPlusBTrialAMD64)
	if trial != nil || !errors.Is(err, problemvalidation.ErrCheckerCandidateBinding) {
		t.Fatalf("offline profile must be refused before even reading a bundle: %v", err)
	}
}

func TestAPlusBJudgeManifestAndSubmissionBinding(t *testing.T) {
	// Synthetic bundle locks are intentionally not admitted by this entry point.
	_, prepared := fixture(t)
	ctx := t.Context()
	plan, err := prepared.PrepareReference(ctx, problem.SmallReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	caps := judging.TextLimits{Bytes: 262144, TokenBytes: 12}
	c, err := problemvalidation.PreflightCheckerCandidate(ctx, prepared, plan.Digest(), judging.LibraryCheckerTextVersion, caps)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := problemvalidation.PrepareAPlusBJudgeManifest(ctx, prepared, plan.Digest(), c.Digest(), caps); !errors.Is(err, problemvalidation.ErrCheckerCandidateBinding) {
		t.Fatalf("a matching self-pinned plan under an untrusted lock was not rejected: %v", err)
	}
	sha := strings.Repeat("a", 64)
	for _, source := range []string{"", "\xff", "int\x00main(){}"} {
		sum := sha256.Sum256([]byte(source))
		if _, err := problemvalidation.PrepareAPlusBJudge(ctx, prepared, plan.Digest(), c.Digest(), sha, hex.EncodeToString(sum[:]), source, caps); !errors.Is(err, problemvalidation.ErrCheckerCandidateBinding) {
			t.Fatalf("invalid or differently hashed source was not rejected: %v", err)
		}
	}
}

func TestIsolatedJudgeCompletionRequiresStopAndBothBindings(t *testing.T) {
	sha := strings.Repeat("a", 64)
	for _, verdict := range []string{"AC", "WA", "CE", "OLE"} {
		report := problemvalidation.CheckerTrialReport{
			Schema: "oj.aplusb-isolated-judge-report.v1", Verdict: verdict,
			JudgeManifestSHA256: sha, SubmissionSHA256: sha,
		}
		if err := report.Finalize(nil, nil, nil); err != nil || !report.Complete || report.Verdict != verdict {
			t.Fatalf("conclusive %s not finalized: %+v %v", verdict, report, err)
		}
		if err := report.Finalize(nil, errors.New("stop unknown"), nil); err == nil || report.Complete || report.StopConfirmed || report.Verdict != "SE" {
			t.Fatalf("unconfirmed stop was published: %+v %v", report, err)
		}
		report.Verdict, report.SubmissionSHA256 = verdict, ""
		if err := report.Finalize(nil, nil, nil); err == nil || report.Complete {
			t.Fatalf("missing submission binding was published: %+v %v", report, err)
		}
	}
}

package problemreference

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestLocalAPlusBTrialNeverWaivesReviewGates(t *testing.T) {
	good := ReviewEligibilitySnapshot{Evidence: ReviewEvidence{
		SourceID: "library-checker:aplusb", SourceLockSHA256: problemvalidation.SourceLockSHA256,
		ContentVersion:   "74c7797f5484daf9df01b1be2e5a537eba7b05b1ef20efc08110c61e3c1490fd",
		ImportRevision:   "e96d280c89a6e5d3da76ee5347c0581d4153d7052e9afceddffb2fdbb3d2036d",
		FormalPlanSHA256: problemvalidation.ReviewedAPlusBPlan,
		ProfileSHA256:    "658fef827daa872a726e6a728ad05c84a08103bd9857c71606fe76cff92c1437", FormalVerdict: "AC",
	}, StatementConfirmed: true, SolutionConfirmed: true}
	got := assessReviewEligibility(good)
	if got.LocalChecker == nil || got.LocalChecker.Scope != "isolated-desktop" ||
		got.LocalChecker.Points != 17 || got.Eligible ||
		!slices.Equal(got.Blockers, []string{"checker_admission_missing", "target_runtime_missing"}) {
		t.Fatal("local archive should be visible without waiving production or target admission")
	}
	subject := strings.Repeat("a", 32)
	good.Evidence.Reviewer, good.Evidence.ReviewID, good.Evidence.DraftID, good.Evidence.ReferenceRequestID = subject, subject, subject, subject
	good.Evidence.AssignmentRevision = 1
	good.Evidence.EvidenceSHA256 = strings.Repeat("c", 64)
	bound, err := bindReviewEligibility(identity.ProblemActor{SubjectID: subject, SessionVersion: identity.SessionVersion{
		RecoveryID: strings.Repeat("b", 32), AuthGeneration: 1,
	}}, good)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(bound)
	if err != nil || len(raw) > 4096 || bound.LocalChecker == nil || len(bound.SnapshotSHA256) != 64 {
		t.Fatalf("full A+B response exceeded the HTTP budget or lost evidence, size=%d err=%v", len(raw), err)
	}
	for _, mutate := range []func(*ReviewEligibilitySnapshot){
		func(s *ReviewEligibilitySnapshot) { s.Returned = true },
		func(s *ReviewEligibilitySnapshot) { s.Evidence.FormalVerdict = "WA" },
		func(s *ReviewEligibilitySnapshot) { s.Evidence.ComparisonOutcome = "passed" },
		func(s *ReviewEligibilitySnapshot) { s.Evidence.ImportRevision = strings.Repeat("1", 64) },
		func(s *ReviewEligibilitySnapshot) { s.Evidence.ContentVersion = strings.Repeat("2", 64) },
		func(s *ReviewEligibilitySnapshot) { s.Evidence.FormalPlanSHA256 = strings.Repeat("3", 64) },
		func(s *ReviewEligibilitySnapshot) { s.Evidence.ProfileSHA256 = strings.Repeat("4", 64) },
		func(s *ReviewEligibilitySnapshot) { s.Evidence.SourceLockSHA256 = strings.Repeat("5", 64) },
	} {
		changed := good
		mutate(&changed)
		got := assessReviewEligibility(changed)
		if got.LocalChecker != nil || got.Eligible ||
			!slices.Contains(got.Blockers, "checker_admission_missing") || !slices.Contains(got.Blockers, "target_runtime_missing") {
			t.Fatal("changed binding should not expose historical local evidence")
		}
	}
}

package problemreference

import (
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

func TestEligibilitySnapshotBindsCurrentActorAndObservedEvidence(t *testing.T) {
	id := strings.Repeat("a", 32)
	digest := strings.Repeat("b", 64)
	actor := identity.ProblemActor{SubjectID: id, SessionVersion: identity.SessionVersion{
		AuthGeneration: 1, RecoveryID: strings.Repeat("e", 32),
	}}
	snap := ReviewEligibilitySnapshot{Evidence: ReviewEvidence{
		Reviewer: id, ReviewID: id, DraftID: id, ReferenceRequestID: id,
		AssignmentRevision: 1, SourceID: "library-checker:aplusb", FormalVerdict: "AC",
		ContentVersion: digest, ImportRevision: digest, SourceLockSHA256: digest,
		EvidenceSHA256: digest, FormalPlanSHA256: digest, ProfileSHA256: digest,
	}}
	baseline, err := bindReviewEligibility(actor, snap)
	if err != nil || !problem.ValidImportRevision(baseline.SnapshotSHA256) || baseline.Eligible {
		t.Fatalf("could not bind a valid blocked assessment: %v", err)
	}
	for _, tc := range []struct {
		name string
		edit func(*identity.ProblemActor, *ReviewEligibilitySnapshot)
	}{
		{"session generation", func(a *identity.ProblemActor, _ *ReviewEligibilitySnapshot) { a.AuthGeneration++ }},
		{"recovery generation", func(a *identity.ProblemActor, _ *ReviewEligibilitySnapshot) { a.RecoveryID = strings.Repeat("c", 32) }},
		{"assigned reviewer", func(a *identity.ProblemActor, s *ReviewEligibilitySnapshot) {
			a.SubjectID = strings.Repeat("c", 32)
			s.Evidence.Reviewer = a.SubjectID
		}},
		{"assignment", func(_ *identity.ProblemActor, s *ReviewEligibilitySnapshot) { s.Evidence.AssignmentRevision++ }},
		{"formal plan", func(_ *identity.ProblemActor, s *ReviewEligibilitySnapshot) {
			s.Evidence.FormalPlanSHA256 = strings.Repeat("c", 64)
		}},
		{"author confirmation", func(_ *identity.ProblemActor, s *ReviewEligibilitySnapshot) { s.SolutionConfirmed = true }},
		{"reviewer confirmation", func(_ *identity.ProblemActor, s *ReviewEligibilitySnapshot) { s.StatementConfirmed = true }},
		{"same-material return", func(_ *identity.ProblemActor, s *ReviewEligibilitySnapshot) { s.Returned = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changedActor, changedSnap := actor, snap
			tc.edit(&changedActor, &changedSnap)
			got, err := bindReviewEligibility(changedActor, changedSnap)
			if err != nil || !problem.ValidImportRevision(got.SnapshotSHA256) || got.SnapshotSHA256 == baseline.SnapshotSHA256 || got.Eligible {
				t.Fatalf("did not bind the changed state or waived a blocker: %v", err)
			}
		})
	}
	snap.Evidence.Reviewer = strings.Repeat("c", 32)
	got, err := bindReviewEligibility(actor, snap)
	if !errors.Is(err, ErrInvalid) || got.SnapshotSHA256 != "" {
		t.Fatal("minted a snapshot for another reviewer")
	}
}

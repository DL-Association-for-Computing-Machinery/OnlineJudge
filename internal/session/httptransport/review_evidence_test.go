package httptransport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type unavailableEvidence struct{ err error }

type eligibilityEvidence struct {
	unavailableEvidence
	status problemreference.ReviewEligibility
}

func (e eligibilityEvidence) EligibilityForSubject(context.Context, redisstore.Credential, string, string) (problemreference.ReviewEligibility, error) {
	return e.status, nil
}

func (e unavailableEvidence) EvidenceForSubject(context.Context, redisstore.Credential, string, string) (problemreference.ReviewEvidence, error) {
	return problemreference.ReviewEvidence{}, e.err
}

func (e unavailableEvidence) ComparisonForSubject(context.Context, redisstore.Credential, string, string) (problemreference.ReviewComparison, error) {
	return problemreference.ReviewComparison{}, e.err
}

func (e unavailableEvidence) EligibilityForSubject(context.Context, redisstore.Credential, string, string) (problemreference.ReviewEligibility, error) {
	return problemreference.ReviewEligibility{}, e.err
}

func TestReviewEvidenceSessionFailurePriority(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{
		{identity.ErrConflict, 401},
		{errors.Join(identity.ErrConflict, identity.ErrStore), 503},
		{errors.Join(identity.ErrConflict, context.Canceled), 503},
	} {
		h, err := transport.WithReviewEvidence(draftHandler(t, &draftUseCase{}), unavailableEvidence{tc.err}, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		r := request(http.MethodGet, "/api/v1/problem-review/"+draftID+"/evidence")
		r.Header.Set("X-OJ-Expected-Subject", strings.Repeat("e", 32))
		response(t, h, r, tc.code, false)
	}
}

func TestEligibilitySeparatesPassingPolicyFromApprovalOptIn(t *testing.T) {
	subject, digest := strings.Repeat("e", 32), strings.Repeat("a", 64)
	for _, tc := range []struct {
		name              string
		enabled, eligible bool
		blockers          []string
		code              int
	}{
		{"passing policy without writes", false, true, nil, 200},
		{"passing policy with writes", true, true, nil, 200},
		{"blocked policy with writes", true, false, []string{"target_runtime_missing"}, 200},
		{"inconsistent policy", true, true, []string{"target_runtime_missing"}, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status := problemreference.ReviewEligibility{
				Evidence: problemreference.ReviewEvidence{Reviewer: subject, ReviewID: draftID, DraftID: draftID,
					AssignmentRevision: 1, SourceID: "synthetic:aplusb", ReferenceRequestID: draftID, FormalVerdict: "AC",
					ContentVersion: digest, ImportRevision: digest, SourceLockSHA256: digest, FormalPlanSHA256: digest,
					ProfileSHA256: digest, EvidenceSHA256: digest},
				PolicyVersion: problemreference.ReviewPolicyVersion, SnapshotSHA256: digest, Eligible: tc.eligible, Blockers: tc.blockers,
			}
			h, err := transport.WithReviewEvidence(draftHandler(t, &draftUseCase{}), eligibilityEvidence{status: status}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.enabled {
				h, err = transport.WithReviewApprovals(h, &approvalFixture{})
				if err != nil {
					t.Fatal(err)
				}
			}
			r := request(http.MethodGet, "/api/v1/problem-review/"+draftID+"/eligibility")
			r.Header.Set("X-OJ-Expected-Subject", subject)
			w := response(t, h, r, tc.code, false)
			if tc.code != 200 {
				return
			}
			var payload struct {
				Eligible        bool     `json:"eligible"`
				ApprovalEnabled *bool    `json:"approval_enabled"`
				Blockers        []string `json:"blockers"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Eligible != tc.eligible || payload.Blockers == nil || len(payload.Blockers) != len(tc.blockers) ||
				(tc.enabled && (payload.ApprovalEnabled == nil || !*payload.ApprovalEnabled)) ||
				(!tc.enabled && payload.ApprovalEnabled != nil) {
				t.Fatalf("unexpected eligibility response: %s", w.Body.String())
			}
		})
	}
}

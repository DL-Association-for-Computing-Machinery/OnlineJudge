//go:build integration

package sessionprotocol_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
)

func assertEntryReviewDiscovery(t *testing.T, endpoint *loginEndpoint, cookies []*http.Cookie, subject, id, revision string) {
	t.Helper()
	r := reviewReadGET(endpoint, "assignments", cookies, subject)
	loginStatus(t, r, 200)
	var page problemreference.ReviewDiscoveryPage
	var wire struct {
		Assignments []map[string]json.RawMessage `json:"assignments"`
	}
	must(t, json.Unmarshal(r.body, &page))
	must(t, json.Unmarshal(r.body, &wire))
	if page.Assignments == nil || page.NextCursor != "" {
		t.Fatal("assignment list is not a complete, bounded page")
	}
	if id == "" {
		if len(page.Assignments) != 0 {
			t.Fatal("unassigned reviewer saw private rows")
		}
		return
	}
	if len(page.Assignments) != 1 || len(wire.Assignments[0]) != 4 || page.Assignments[0].ReviewID != id ||
		page.Assignments[0].ImportRevision != revision || !page.Assignments[0].Valid() {
		t.Fatal("assignment list lost the material binding or exposed extra fields")
	}
}

func assertReviewDiscoveryCrossReplica(t *testing.T, f *fixture, x, y *loginEndpoint, store *referencepg.ReviewStore,
	cookies []*http.Cookie, subject, existingID string) {
	t.Helper()
	// The direct and proxy subtests share a database; use the freshly created
	// reviewer as the fixture ID namespace.
	base := "00" + subject[len(subject)-26:]
	_, err := f.pools[0].Exec(f.ctx, `INSERT INTO problem_review_submissions
		(id,owner_subject_id,request_key,draft_id,source_id,content_version,import_revision,source_lock_sha256,
		reference_request_id,formal_plan_sha256,profile_sha256,evidence_sha256,formal_verdict,
		differential_plan_sha256,differential_outcome,auth_generation,recovery_id,author_revision)
		SELECT $2::text||lpad(to_hex(n),4,'0'),owner_subject_id,'review-discovery-http-'||$2||'-'||n,draft_id,source_id,content_version,
		import_revision,source_lock_sha256,reference_request_id,formal_plan_sha256,profile_sha256,evidence_sha256,
		formal_verdict,differential_plan_sha256,differential_outcome,auth_generation,recovery_id,author_revision
		FROM problem_review_submissions CROSS JOIN generate_series(1,21) n WHERE id=$1`, existingID, base)
	must(t, err)
	for n := 1; n <= 21; n++ {
		id := fmt.Sprintf("%s%04x", base, n)
		_, err = store.SetAssignment(f.ctx, problemreference.ReviewAssignmentChange{ReviewID: id,
			ReviewerSubjectID: subject, ExpectedReviewerRevision: 1, Operator: "entry-fixture", Reason: "synthetic cursor candidate"})
		must(t, err)
		_, err = store.SetAssignment(f.ctx, problemreference.ReviewAssignmentChange{ReviewID: id, ExpectedRevision: 1,
			Operator: "entry-fixture", Reason: "withdraw synthetic cursor candidate"})
		must(t, err)
	}
	result := reviewReadGET(x, "assignments", cookies, subject)
	loginStatus(t, result, 200)
	var page problemreference.ReviewDiscoveryPage
	must(t, json.Unmarshal(result.body, &page))
	if page.Assignments == nil || len(page.Assignments) > 1 || len(page.NextCursor) != 91 {
		t.Fatal("historical candidates must yield a sealed cursor without appearing as assignments")
	}
	first := page.Assignments
	result = reviewReadGET(y, "assignments?cursor="+page.NextCursor, cookies, subject)
	loginStatus(t, result, 200)
	must(t, json.Unmarshal(result.body, &page))
	combined := append(first, page.Assignments...)
	if page.Assignments == nil || len(combined) != 1 || combined[0].ReviewID != existingID || page.NextCursor != "" {
		t.Fatal("reverse proxy replica could not resume the direct instance's cursor")
	}
}

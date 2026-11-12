//go:build integration && linux

package integration_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func TestReferenceReservationFencedAfterArchiveRestore(t *testing.T) {
	f, _, node, report := acceptedReportFixture(t, "AC")
	archive, schema := dumpSubmissionFixture(t, f.backlogFixture, t.TempDir())
	target := newSubmissionRestoreTarget(t, f.ctx, schema, archive)
	identities, err := identitypg.New(target, identityHasher(t, false))
	must(t, err)
	authors, err := identitypg.NewProblemAccess(target)
	must(t, err)
	access, err := problempg.NewReferenceAccess(authors)
	must(t, err)
	store, err := referencepg.New(target, access)
	must(t, err)
	claims, err := referencepg.NewReferenceClaimer(store, submissionpg.LockReferenceNode, node.ID, f.request.Specification.Identity.SourceLockSHA256)
	must(t, err)
	input := problemreference.MaterialInput{ClaimInput: report.ClaimInput, Token: report.Token}

	// A restore alone revives the archived authority. Prove it while the
	// original credential and reservation still work, then fence the target.
	grant, err := claims.AuthorizeReferenceMaterials(f.ctx, node, input)
	must(t, err)
	if grant.Plan.SHA256 != f.want.SHA256 || !bytes.Equal(grant.Plan.Encoding, f.want.Encoding) || grant.Deadline.IsZero() {
		t.Fatal("restored authorization did not return the archived plan")
	}
	renewed, err := claims.RenewReference(f.ctx, node, input)
	must(t, err)
	if !renewed.ValidFor(report.ClaimInput) || renewed.Token != report.Token {
		t.Fatal("restored reservation could not renew with the archived token")
	}

	epoch, err := identities.BeginRecovery(f.ctx)
	must(t, err)
	if epoch == f.actor.RecoveryID {
		t.Fatal("closing recovery did not replace the archived identity epoch")
	}
	assertFenced := func(t *testing.T) {
		t.Helper()
		if got, err := claims.AuthorizeReferenceMaterials(f.ctx, node, input); got.Plan.Encoding != nil || !got.Deadline.IsZero() || !errors.Is(err, problemreference.ErrClaimUnavailable) {
			t.Fatalf("old reservation could still read material: %v", err)
		}
		if got, err := claims.RenewReference(f.ctx, node, input); got != (problemreference.WorkerClaim{}) || !errors.Is(err, problemreference.ErrClaimUnavailable) {
			t.Fatalf("old reservation could still renew: %v", err)
		}
		if err := claims.ReportReference(f.ctx, node, report); !errors.Is(err, problemreference.ErrClaimUnavailable) {
			t.Fatalf("old reservation could still submit its first report: %v", err)
		}

		var state string
		var reports int
		must(t, target.QueryRow(f.ctx, `SELECT state, (SELECT count(*) FROM problem_reference_worker_reports WHERE request_id=$1)
			FROM problem_reference_requests WHERE id=$1`, report.RequestID).Scan(&state, &reports))
		var until time.Time
		var unchanged, live bool
		must(t, target.QueryRow(f.ctx, `SELECT lease_until, request_id=$1 AND lease_token=$2, lease_until>clock_timestamp()
			FROM problem_reference_worker_claim WHERE singleton`, report.RequestID, report.Token).Scan(&until, &unchanged, &live))
		if state != string(problemreference.Queued) || reports != 0 || !unchanged || !until.Equal(renewed.Until) {
			t.Fatal("rejected operations changed the request, report ledger, or reservation")
		}
		if !live {
			t.Fatal("reservation expired before the rejection assertions finished")
		}
	}
	t.Run("closed", assertFenced)
	must(t, identities.FinishRecovery(f.ctx, epoch))
	t.Run("reopened", assertFenced)

	// Simulate the passage of the lease deadline so a busy singleton cannot
	// account for rejection of an explicit attempt to reclaim the old request.
	_, err = target.Exec(f.ctx, `UPDATE problem_reference_worker_claim SET lease_until=clock_timestamp()-interval '1 second' WHERE singleton`)
	must(t, err)
	if got, err := claims.ClaimReference(f.ctx, node, report.ClaimInput); got != (problemreference.WorkerClaim{}) || !errors.Is(err, problemreference.ErrClaimUnavailable) {
		t.Fatalf("old request could still be claimed after its reservation expired: %v", err)
	}
}

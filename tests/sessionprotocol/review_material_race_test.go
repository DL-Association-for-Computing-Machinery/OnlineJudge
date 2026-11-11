//go:build integration

package sessionprotocol_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type reviewReadHook struct {
	*materialstore.Local
	opened  int
	trigger int
	after   func(materialstore.Object)
}

func (h *reviewReadHook) Open(ctx context.Context, obj materialstore.Object) (*materialstore.Snapshot, error) {
	snapshot, err := h.Local.Open(ctx, obj)
	if err == nil {
		h.opened++
		// The hook runs only after the selected object's bytes are snapshotted.
		if h.opened == h.trigger && h.after != nil {
			h.after(obj)
		}
	}
	return snapshot, err
}

func testReviewMaterialRaces(t *testing.T, f *fixture, access *identitypg.ProblemAccess, store *referencepg.ReviewStore,
	root, namespace, reviewer string, credential protocol.Credential, receipt problemreference.ReviewReceipt) {
	t.Helper()
	objects, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
	must(t, err)
	defer func() { must(t, objects.Close()) }()
	sessions := f.store(t, namespace, 0, protocol.OrdinaryPolicy())
	repo, err := referencepg.NewReviewMaterialStore(store, problempg.ReadBoundReviewDraft)
	must(t, err)
	var roleRev, assignmentRev int64 = 3, 1
	assign := func(t *testing.T, who string, rev int64) {
		t.Helper()
		a, err := store.SetAssignment(f.ctx, problemreference.ReviewAssignmentChange{ReviewID: receipt.ID,
			ExpectedRevision: assignmentRev, ReviewerSubjectID: who, ExpectedReviewerRevision: rev,
			Operator: "review-read-fixture", Reason: "exercise material read boundary"})
		must(t, err)
		assignmentRev = a.Revision
	}
	grant := func(t *testing.T, enabled bool) {
		t.Helper()
		r, err := access.SetReviewerRole(f.ctx, identity.ProblemRoleChange{SubjectID: reviewer, ExpectedRevision: roleRev, Enabled: enabled,
			Operator: "review-read-fixture", Reason: "exercise material read boundary"})
		must(t, err)
		roleRev = r.Revision
	}
	assign(t, reviewer, roleRev)
	run := func(t *testing.T, effect func(materialstore.Object), want error) {
		t.Helper()
		held := false
		h := &reviewReadHook{Local: objects, trigger: 4, after: effect}
		s, err := problemreference.NewReviewMaterialReader(sessions, repo, h, func() (func(), error) {
			if held {
				return nil, problem.ErrBusy
			}
			held = true
			return func() { held = false }, nil
		})
		must(t, err)
		ctx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
		defer cancel()
		result, err := s.MaterialsForSubject(ctx, credential, reviewer, receipt.ID)
		if !errors.Is(err, want) || !reflect.DeepEqual(result, problemreference.ReviewMaterials{}) || held || h.opened != 4 {
			t.Fatalf("private output, slot leak or wrong read boundary (opened=%d, error=%v, want=%v)", h.opened, err, want)
		}
	}
	t.Run("role_revoke_regrant_and_reassign", func(t *testing.T) {
		run(t, func(_ materialstore.Object) {
			grant(t, false)
			grant(t, true)
			assign(t, reviewer, roleRev)
		}, problemreference.ErrNotFound)
	})
	t.Run("withdraw_and_reassign", func(t *testing.T) {
		run(t, func(_ materialstore.Object) {
			assign(t, "", 0)
			assign(t, reviewer, roleRev)
		}, problemreference.ErrNotFound)
	})
	t.Run("same_content_different_import_revision", func(t *testing.T) {
		change := func(rev string) {
			t.Helper()
			tag, err := f.pools[1].Exec(f.ctx, `UPDATE problem_drafts SET request_sha256=$1 WHERE id=$2 AND content_version=$3`, rev, receipt.DraftID, receipt.ContentVersion)
			must(t, err)
			if tag.RowsAffected() != 1 {
				t.Fatal("test did not change the revision at the same content version")
			}
		}
		defer change(receipt.ImportRevision)
		run(t, func(_ materialstore.Object) { change(strings.Repeat("f", 64)) }, problemreference.ErrStore)
	})
	t.Run("logout_after_final_object", func(t *testing.T) {
		run(t, func(_ materialstore.Object) { must(t, sessions.Revoke(f.ctx, credential)) }, protocol.ErrStale)
	})
}

func testStatementLogoutAfterObjects(t *testing.T, f *fixture, store *referencepg.ReviewStore, e problemreference.ReviewEvidence,
	root, namespace, reviewer string, credential protocol.Credential) {
	t.Helper()
	objects, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
	must(t, err)
	defer func() { must(t, objects.Close()) }()
	sessions := f.store(t, namespace, 0, protocol.OrdinaryPolicy())
	repo, err := referencepg.NewReviewMaterialStore(store, problempg.ReadBoundReviewDraft)
	must(t, err)
	// The confirmation reads statement and both sample objects, not the solver.
	h := &reviewReadHook{Local: objects, trigger: 3, after: func(_ materialstore.Object) {
		must(t, sessions.Revoke(f.ctx, credential))
	}}
	released := false
	s, err := problemreference.NewStatementConfirmations(sessions, repo, h, func() (func(), error) {
		return func() { released = true }, nil
	})
	must(t, err)
	i := problemreference.StatementConfirmationIntent{RequestKey: "statement-logout-after-objects",
		AssignmentRevision: e.AssignmentRevision, ImportRevision: e.ImportRevision,
		EvidenceSHA256: e.EvidenceSHA256, ChecklistVersion: problemreference.StatementChecklistVersion}
	result, err := s.ConfirmForSubject(f.ctx, credential, reviewer, e.ReviewID, i)
	if !errors.Is(err, protocol.ErrStale) || result != (problemreference.StatementConfirmationReceipt{}) || h.opened != 3 || !released {
		t.Fatalf("logout after objects did not fence the confirmation (opened=%d, released=%v, err=%v)", h.opened, released, err)
	}
	var count int
	must(t, f.pools[0].QueryRow(f.ctx, `SELECT count(*) FROM problem_statement_confirmations WHERE review_id=$1`, e.ReviewID).Scan(&count))
	if count != 0 {
		t.Fatal("logged-out confirmation wrote to the ledger")
	}
}

// Corrupting one private fixture object must fail before a DTO is assembled,
// even if preceding statement objects were valid and already read.
func corruptReviewSource(t *testing.T, root, hash string) func() {
	t.Helper()
	if !problem.ValidImportRevision(hash) {
		t.Fatal("fixture has invalid solution object identity")
	}
	path := filepath.Join(root, hash)
	original, err := os.ReadFile(path)
	must(t, err)
	if len(original) == 0 {
		t.Fatal("fixture solution is empty")
	}
	changed := append([]byte(nil), original...)
	changed[0] ^= 1
	must(t, os.WriteFile(path, changed, 0600))
	return func() { must(t, os.WriteFile(path, original, 0600)) }
}

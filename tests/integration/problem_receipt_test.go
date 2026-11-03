//go:build integration

package integration_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
)

func TestImportReceiptOwnerAndCurrentRole(t *testing.T) {
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	key := "receipt-owner-original-key"
	draft, err := f.repository.Create(f.ctx, f.actor, key, f.prepared)
	must(t, err)
	got, err := f.repository.Receipt(f.ctx, f.actor, key)
	must(t, err)
	if got.DraftID != draft.ID || got.ImportRevision != f.prepared.Content().RequestSHA256 || got.Version != draft.Version || got.SourceID != draft.SourceID || got.RequestKey != key || got.CreatedAt != draft.CreatedAt || got.State != "private-draft" || got.ValidationState != "not-run" {
		t.Fatal("receipt did not represent the committed import")
	}
	raw, err := json.Marshal(got)
	must(t, err)
	var fields map[string]any
	must(t, json.Unmarshal(raw, &fields))
	if len(fields) != 8 || fields["document"] != nil || fields["references"] != nil || fields["owner"] != nil {
		t.Fatal("receipt became material or identity metadata")
	}
	other := f.actor
	other.SubjectID = identityID(1101)
	f.role(t, other.SubjectID, 0, true)
	for _, candidate := range []string{key, "no-such-receipt-original-key"} {
		result, e := f.repository.Receipt(f.ctx, other, candidate)
		if !errors.Is(e, problem.ErrNotFound) || result != (problem.ImportReceipt{}) {
			t.Fatal("other owner observed receipt existence", e)
		}
	}
	otherDraft, err := f.repository.Create(f.ctx, other, key, f.prepared)
	must(t, err)
	otherReceipt, err := f.repository.Receipt(f.ctx, other, key)
	must(t, err)
	if otherReceipt.DraftID != otherDraft.ID || otherReceipt.DraftID == got.DraftID {
		t.Fatal("same request key crossed owner scope")
	}
	f.role(t, f.actor.SubjectID, 1, false)
	for _, candidate := range []string{key, "no-such-receipt-original-key"} {
		result, e := f.repository.Receipt(f.ctx, f.actor, candidate)
		if !errors.Is(e, identity.ErrForbidden) || result != (problem.ImportReceipt{}) {
			t.Fatal("receipt lookup bypassed current author role", e)
		}
	}
}

func TestImportReceiptMissingDoesNotCancelInFlightCreate(t *testing.T) {
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	held := heldDraftAccess{access: f.access, locked: make(chan uint32, 1), release: make(chan struct{}), fixture: f}
	repository, err := problempg.New(f.a, held)
	must(t, err)
	const key = "in-flight-original-receipt-key"
	completed := make(chan error, 1)
	go func() { _, e := repository.Create(f.ctx, f.actor, key, f.prepared); completed <- e }()
	select {
	case <-held.locked:
	case e := <-completed:
		t.Fatalf("create failed before the barrier: %v", e)
	case <-f.ctx.Done():
		t.Fatal("create barrier timed out")
	}
	missing, err := f.repository.Receipt(f.ctx, f.actor, key)
	if !errors.Is(err, problem.ErrNotFound) || missing != (problem.ImportReceipt{}) {
		t.Fatal("missing observation did not remain empty", err)
	}
	close(held.release)
	must(t, <-completed)
	got, err := f.repository.Receipt(f.ctx, f.actor, key)
	must(t, err)
	if got.RequestKey != key || got.ImportRevision != f.prepared.Content().RequestSHA256 {
		t.Fatal("in-flight request did not later commit under original key")
	}
	f.count(t, 1, len(f.prepared.Content().References))
	t.Log("receipt was absent while an authorized create was in flight, then visible after commit; absence is not cancellation proof")
}

func TestImportReceiptAuthorityUsesTheReadTransaction(t *testing.T) {
	for _, mode := range []string{"role", "account", "recovery", "lost-transaction"} {
		t.Run(mode, func(t *testing.T) {
			f := newDraftFixture(t)
			f.role(t, f.actor.SubjectID, 0, true)
			const key = "receipt-revocation-original-key"
			_, err := f.repository.Create(f.ctx, f.actor, key, f.prepared)
			must(t, err)
			held := heldDraftAccess{access: f.access, locked: make(chan uint32, 1), release: make(chan struct{}), fixture: f, lose: mode == "lost-transaction"}
			repository, err := problempg.New(f.a, held)
			must(t, err)
			if held.lose {
				got, e := repository.Receipt(f.ctx, f.actor, key)
				if !errors.Is(e, problem.ErrStore) || got != (problem.ImportReceipt{}) {
					t.Fatal("receipt escaped its dead authorized transaction", e)
				}
				return
			}
			read := make(chan error, 1)
			go func() { _, e := repository.Receipt(f.ctx, f.actor, key); read <- e }()
			var pid uint32
			select {
			case pid = <-held.locked:
			case e := <-read:
				t.Fatalf("read failed before locks: %v", e)
			case <-f.ctx.Done():
				t.Fatal("read barrier timed out")
			}
			revoked := make(chan error, 1)
			go func() {
				var e error
				switch mode {
				case "role":
					_, e = f.access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: f.actor.SubjectID, ExpectedRevision: 1, Enabled: false, Operator: "receipt-fixture", Reason: "concurrent receipt revocation"})
				case "account":
					_, e = f.identities.DisableCAS(f.ctx, f.actor.SubjectID, identity.Revision{CredentialVersion: 1, AuthGeneration: 1})
				case "recovery":
					_, e = f.identities.BeginRecovery(f.ctx)
				}
				revoked <- e
			}()
			waitBacklogBlocked(t, f.backlogFixture, pid, f.appA)
			close(held.release)
			must(t, <-read)
			must(t, <-revoked)
			got, err := f.repository.Receipt(f.ctx, f.actor, key)
			if err == nil || got != (problem.ImportReceipt{}) {
				t.Fatal("stale authority read a receipt after revocation")
			}
		})
	}
}

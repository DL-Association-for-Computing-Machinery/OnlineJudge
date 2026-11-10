//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
)

type referenceRequestFixture struct {
	draftFixture
	store   *referencepg.Store
	catalog *problemreference.Catalog
	draft   problem.Draft
}

func newReferenceRequestFixture(t *testing.T) referenceRequestFixture {
	t.Helper()
	f := referenceRequestFixture{draftFixture: newDraftFixture(t)}
	f.role(t, f.actor.SubjectID, 0, true)
	var err error
	f.draft, err = f.repository.Create(f.ctx, f.actor, "reference-draft-original-key", f.prepared)
	must(t, err)
	f.catalog, err = problemreference.NewCatalog(map[string]*problem.Prepared{f.draft.SourceID: f.prepared})
	must(t, err)
	access, err := problempg.NewReferenceAccess(f.access)
	must(t, err)
	f.store, err = referencepg.New(f.a, access)
	must(t, err)
	return f
}

func (f referenceRequestFixture) limits(t *testing.T, global, owner int64) {
	t.Helper()
	must(t, referencepg.Configure(f.ctx, f.a, problemreference.Limits{Global: global, Owner: owner}))
}

func (f referenceRequestFixture) counts(t *testing.T, total, queued int) {
	t.Helper()
	var all, active int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*),count(*) FILTER (WHERE state='queued') FROM problem_reference_requests`).Scan(&all, &active))
	if all != total || active != queued {
		t.Fatalf("reference history/queued=%d/%d want=%d/%d", all, active, total, queued)
	}
}

func noReferenceResult(t *testing.T, r problemreference.Request, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || !reflect.DeepEqual(r, problemreference.Request{}) {
		t.Fatalf("unexpected reference result/error: %v, want %v; zero=%v", err, want, reflect.DeepEqual(r, problemreference.Request{}))
	}
}

func TestReferenceRequestsLifecycleAndOwnership(t *testing.T) {
	f := newReferenceRequestFixture(t)
	key := "reference-original-intent-01"
	r, err := f.store.Enqueue(f.ctx, f.actor, key, f.draft.ID, f.catalog)
	noReferenceResult(t, r, err, problemreference.ErrPolicy)
	f.counts(t, 0, 0)
	f.limits(t, 2, 1)
	first, err := f.store.Enqueue(f.ctx, f.actor, key, f.draft.ID, f.catalog)
	must(t, err)
	if first.State != problemreference.Queued || first.CancelledAt != nil || first.DraftID != f.draft.ID || first.Authorization != (identity.ProblemAuthorSnapshot{Actor: f.actor, RoleRevision: 1}) {
		t.Fatal("request did not bind original draft and authority")
	}
	if first.Specification.Identity.Version != f.draft.Version || first.Specification.Identity.ImportRevision != f.draft.ImportRevision || !first.Specification.Valid() {
		t.Fatal("stored configuration identity changed")
	}
	f.counts(t, 1, 1)
	r, err = f.store.Enqueue(f.ctx, f.actor, "reference-new-full-queue", f.draft.ID, f.catalog)
	noReferenceResult(t, r, err, problemreference.ErrFull)
	retired, err := problemreference.NewCatalog(nil)
	must(t, err)
	recovered, err := f.store.Enqueue(f.ctx, f.actor, key, f.draft.ID, retired)
	must(t, err)
	if !reflect.DeepEqual(recovered, first) {
		t.Fatal("retirement/full queue rebound an original request")
	}
	// Missing admission must not prevent recovery of an already committed key.
	_, err = f.a.Exec(f.ctx, `DELETE FROM problem_reference_request_policy`)
	must(t, err)
	recovered, err = f.store.Enqueue(f.ctx, f.actor, key, f.draft.ID, retired)
	must(t, err)
	if !reflect.DeepEqual(recovered, first) {
		t.Fatal("missing policy lost the original intent")
	}
	_, err = f.store.Receipt(f.ctx, f.actor, key)
	must(t, err)
	f.limits(t, 2, 1)
	r, err = f.store.Enqueue(f.ctx, f.actor, "reference-new-retired-key", f.draft.ID, retired)
	noReferenceResult(t, r, err, problemreference.ErrSourceChanged)
	secondDraft, err := f.repository.Create(f.ctx, f.actor, "reference-second-draft-key", f.prepared)
	must(t, err)
	r, err = f.store.Enqueue(f.ctx, f.actor, key, secondDraft.ID, f.catalog)
	noReferenceResult(t, r, err, problemreference.ErrConflict)
	other := f.actor
	other.SubjectID = identityID(1101)
	f.role(t, other.SubjectID, 0, true)
	r, err = f.store.Enqueue(f.ctx, other, "reference-foreign-draft-key", f.draft.ID, f.catalog)
	noReferenceResult(t, r, err, problemreference.ErrNotFound)
	r, err = f.store.Receipt(f.ctx, other, key)
	noReferenceResult(t, r, err, problemreference.ErrNotFound)
	r, err = f.store.Cancel(f.ctx, other, key)
	noReferenceResult(t, r, err, problemreference.ErrNotFound)
	for _, absent := range []string{"reference-absent-original", "reference-foreign-key-01"} {
		r, err = f.store.Receipt(f.ctx, f.actor, absent)
		noReferenceResult(t, r, err, problemreference.ErrNotFound)
	}
	f.role(t, f.actor.SubjectID, 1, false)
	for _, op := range []func() (problemreference.Request, error){
		func() (problemreference.Request, error) {
			return f.store.Enqueue(f.ctx, f.actor, key, f.draft.ID, f.catalog)
		},
		func() (problemreference.Request, error) { return f.store.Receipt(f.ctx, f.actor, key) },
		func() (problemreference.Request, error) { return f.store.Cancel(f.ctx, f.actor, key) },
	} {
		r, err = op()
		noReferenceResult(t, r, err, identity.ErrForbidden)
	}
	f.role(t, f.actor.SubjectID, 2, true)
	r, err = f.store.Receipt(f.ctx, f.actor, key)
	must(t, err)
	if !reflect.DeepEqual(r, first) {
		t.Fatal("regrant renewed or changed original authority")
	}
	cancelled, err := f.store.Cancel(f.ctx, f.actor, key)
	must(t, err)
	if cancelled.State != problemreference.Cancelled || cancelled.CancelledAt == nil || cancelled.Authorization != first.Authorization || !reflect.DeepEqual(cancelled.Specification, first.Specification) {
		t.Fatal("cancellation altered immutable intent")
	}
	again, err := f.store.Cancel(f.ctx, f.actor, key)
	must(t, err)
	if !reflect.DeepEqual(again, cancelled) {
		t.Fatal("repeated cancel changed original receipt")
	}
	current, err := f.store.Enqueue(f.ctx, f.actor, "reference-after-cancel-key", f.draft.ID, f.catalog)
	must(t, err)
	if current.Authorization.RoleRevision != 3 {
		t.Fatal("new intent reused the pre-regrant authority")
	}
	f.counts(t, 2, 1)
	unchanged, err := f.repository.GetOwn(f.ctx, f.actor, f.draft.ID)
	must(t, err)
	if !reflect.DeepEqual(unchanged, f.draft) || unchanged.ValidationState != "not-run" {
		t.Fatal("request lifecycle changed the private draft")
	}
}

func TestReferenceRequestBindsPersistedDraftMaterials(t *testing.T) {
	f := newReferenceRequestFixture(t)
	f.limits(t, 1, 1)
	const key = "reference-bind-materials-01"
	_, err := f.store.Enqueue(f.ctx, f.actor, key, f.draft.ID, f.catalog)
	must(t, err)
	want := f.prepared.Content()
	must(t, f.prepared.Close())

	request, err := f.store.Receipt(f.ctx, f.actor, key)
	must(t, err)
	stored, err := f.repository.GetOwn(f.ctx, f.actor, f.draft.ID)
	must(t, err)
	bound, err := request.BindDraft(stored)
	must(t, err)
	if !reflect.DeepEqual(bound, want) {
		t.Fatal("metadata reloaded from PostgreSQL differs from the original import")
	}

	// Lose evidence without changing the row's version or import revision.
	result, err := f.a.Exec(f.ctx, `DELETE FROM problem_draft_materials WHERE draft_id=$1 AND logical_name=$2`,
		f.draft.ID, "provenance/source-validation.json")
	must(t, err)
	if result.RowsAffected() != 1 {
		t.Fatal("expected one evidence reference in the fixture")
	}
	stored, err = f.repository.GetOwn(f.ctx, f.actor, f.draft.ID)
	must(t, err)
	if stored.Version != want.Version || stored.ImportRevision != want.RequestSHA256 {
		t.Fatal("fixture changed more than the evidence reference")
	}
	if bound, err = request.BindDraft(stored); !errors.Is(err, problem.ErrInvalid) || !reflect.DeepEqual(bound, problem.Content{}) {
		t.Fatal("missing evidence was accepted by the frozen revision", err)
	}
}

func TestReferenceRequestsConcurrentKeys(t *testing.T) {
	t.Run("same key fills final slot", func(t *testing.T) {
		f := newReferenceRequestFixture(t)
		f.limits(t, 1, 1)
		type outcome struct {
			r   problemreference.Request
			err error
		}
		out, start := make(chan outcome, 8), make(chan struct{})
		for range 8 {
			go func() {
				<-start
				r, e := f.store.Enqueue(f.ctx, f.actor, "reference-concurrent-same", f.draft.ID, f.catalog)
				out <- outcome{r, e}
			}()
		}
		close(start)
		var first problemreference.Request
		for range 8 {
			r := <-out
			must(t, r.err)
			if first.ID == "" {
				first = r.r
			} else if !reflect.DeepEqual(first, r.r) {
				t.Fatal("parallel same key returned different receipt")
			}
		}
		f.counts(t, 1, 1)
	})
	t.Run("different drafts same key", func(t *testing.T) {
		f := newReferenceRequestFixture(t)
		f.limits(t, 1, 1)
		other, err := f.repository.Create(f.ctx, f.actor, "reference-different-draft", f.prepared)
		must(t, err)
		out, start := make(chan error, 2), make(chan struct{})
		for _, id := range []string{f.draft.ID, other.ID} {
			go func() {
				<-start
				_, e := f.store.Enqueue(f.ctx, f.actor, "reference-same-key-conflict", id, f.catalog)
				out <- e
			}()
		}
		close(start)
		success, conflict := 0, 0
		for range 2 {
			e := <-out
			if e == nil {
				success++
			} else if errors.Is(e, problemreference.ErrConflict) {
				conflict++
			} else {
				t.Fatal(e)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatal("same key bypassed draft conflict")
		}
		f.counts(t, 1, 1)
	})
	t.Run("different keys respect global and owner caps", func(t *testing.T) {
		f := newReferenceRequestFixture(t)
		f.limits(t, 2, 1)
		other := f.actor
		other.SubjectID = identityID(1101)
		f.role(t, other.SubjectID, 0, true)
		draft, err := f.repository.Create(f.ctx, other, "reference-other-owner-draft", f.prepared)
		must(t, err)
		out, start := make(chan error, 10), make(chan struct{})
		for n := range 10 {
			actor, id := f.actor, f.draft.ID
			if n%2 == 1 {
				actor, id = other, draft.ID
			}
			go func() {
				<-start
				_, e := f.store.Enqueue(f.ctx, actor, fmt.Sprintf("reference-concurrent-key-%02d", n), id, f.catalog)
				out <- e
			}()
		}
		close(start)
		success := 0
		for range 10 {
			e := <-out
			if e == nil {
				success++
			} else if !errors.Is(e, problemreference.ErrFull) {
				t.Fatal(e)
			}
		}
		if success != 2 {
			t.Fatal("concurrent owner/global admission was not exact")
		}
		f.counts(t, 2, 2)
	})
	t.Run("cancel competes with new intent", func(t *testing.T) {
		f := newReferenceRequestFixture(t)
		f.limits(t, 1, 1)
		_, err := f.store.Enqueue(f.ctx, f.actor, "reference-cancel-old-key", f.draft.ID, f.catalog)
		must(t, err)
		cancelled, created := make(chan error, 1), make(chan error, 1)
		go func() { _, e := f.store.Cancel(f.ctx, f.actor, "reference-cancel-old-key"); cancelled <- e }()
		go func() {
			_, e := f.store.Enqueue(f.ctx, f.actor, "reference-cancel-new-key", f.draft.ID, f.catalog)
			created <- e
		}()
		must(t, <-cancelled)
		err = <-created
		if err != nil && !errors.Is(err, problemreference.ErrFull) {
			t.Fatal(err)
		}
		// An explicit retry of metadata admission after the completed cancellation
		// recovers the same key or occupies its newly available slot; no program runs.
		_, err = f.store.Enqueue(f.ctx, f.actor, "reference-cancel-new-key", f.draft.ID, f.catalog)
		must(t, err)
		f.counts(t, 2, 1)
	})
}

func waitReferencePolicyWaiters(t *testing.T, f referenceRequestFixture, count int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, 2*time.Second)
	defer cancel()
	for {
		var blocked int
		must(t, f.b.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock'
            AND query LIKE '%problem_reference_request_policy%' AND cardinality(pg_blocking_pids(pid))>0`, f.appA).Scan(&blocked))
		if blocked == count {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("reference requests did not reach the actual policy wait")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestReferenceRequestsSameKeyAfterPolicyWait(t *testing.T) {
	for _, differentDraft := range []bool{false, true} {
		t.Run(fmt.Sprint(differentDraft), func(t *testing.T) {
			f := newReferenceRequestFixture(t)
			f.limits(t, 1, 1)
			ids := []string{f.draft.ID, f.draft.ID}
			if differentDraft {
				d, e := f.repository.Create(f.ctx, f.actor, "reference-policy-other-draft", f.prepared)
				must(t, e)
				ids[1] = d.ID
			}
			holder, err := f.b.Begin(f.ctx)
			must(t, err)
			defer holder.Rollback(f.ctx)
			var limit int64
			must(t, holder.QueryRow(f.ctx, `SELECT global_limit FROM problem_reference_request_policy WHERE singleton FOR UPDATE`).Scan(&limit))
			type outcome struct {
				r   problemreference.Request
				err error
			}
			out := make(chan outcome, 2)
			for _, id := range ids {
				go func() {
					r, e := f.store.Enqueue(f.ctx, f.actor, "reference-forced-policy-wait", id, f.catalog)
					out <- outcome{r, e}
				}()
			}
			// Both passed the initial missing-key read. The second waiter may wait
			// behind the first tuple waiter, so inspect both live policy queries.
			waitReferencePolicyWaiters(t, f, 2)
			must(t, holder.Commit(f.ctx))
			one, two := <-out, <-out
			if !differentDraft {
				must(t, one.err)
				must(t, two.err)
				if !reflect.DeepEqual(one.r, two.r) {
					t.Fatal("post-wait retry changed original receipt")
				}
			} else if !((one.err == nil && errors.Is(two.err, problemreference.ErrConflict)) || (two.err == nil && errors.Is(one.err, problemreference.ErrConflict))) {
				t.Fatalf("post-wait different draft errors=%v/%v", one.err, two.err)
			}
			f.counts(t, 1, 1)
		})
	}
}

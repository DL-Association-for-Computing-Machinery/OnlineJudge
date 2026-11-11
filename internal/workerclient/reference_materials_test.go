package workerclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func referenceMaterialsFixture(t *testing.T) (problem.ReferencePlan, map[string][]byte) {
	t.Helper()
	b := problemfixture.New(t)
	b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", []byte("synthetic source; never executed\n"))
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	}()
	plan, err := p.PrepareReference(t.Context(), problem.SmallReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	plan.SourceLockSHA256 = problemvalidation.SourceLockSHA256
	plan.SourceData = nil
	for i := range plan.Cases {
		plan.Cases[i].InputData, plan.Cases[i].AnswerData = nil, nil
	}
	objects := make(map[string][]byte)
	for _, raw := range b.Files {
		objects[problemfixture.Hash(raw)] = bytes.Clone(raw)
	}
	return plan, objects
}

func TestFetchReferenceDeadlineIncludesDescriptorRoundTrip(t *testing.T) {
	plan, objects := referenceMaterialsFixture(t)
	claim, encoding := referenceClaim(t, plan)
	var calls atomic.Int32
	base := referenceHandler(t, claim, encoding, 30, objects, "", &calls)
	a := acquisitionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/worker/reference/assignment" {
			time.Sleep(100 * time.Millisecond)
		}
		base.ServeHTTP(w, r)
	}))
	got, err := a.FetchReference(t.Context(), claim)
	if got != nil || !errors.Is(err, ErrMaterials) || calls.Load() != 1 {
		t.Fatal("expired descriptor accepted or object requested", err, calls.Load())
	}
}

func TestFetchReferenceSnapshotExpires(t *testing.T) {
	plan, objects := referenceMaterialsFixture(t)
	claim, encoding := referenceClaim(t, plan)
	var calls atomic.Int32
	a := acquisitionServer(t, referenceHandler(t, claim, encoding, 500, objects, "", &calls))
	start := time.Now()
	got, err := a.FetchReference(t.Context(), claim)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := got.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := got.ReadSource(t.Context()); err != nil {
		t.Fatal("snapshot unreadable before deadline", err)
	}
	time.Sleep(time.Until(start.Add(550 * time.Millisecond)))
	if raw, err := got.ReadSource(t.Context()); err == nil || raw != nil {
		t.Fatal("snapshot readable after conservative deadline", err)
	}
}

func TestFetchReferenceAssignmentStatusDoesNotRetry(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{{http.StatusUnauthorized, submission.ErrNode}, {http.StatusConflict, problemreference.ErrClaimUnavailable}} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			plan, _ := referenceMaterialsFixture(t)
			claim, _ := referenceClaim(t, plan)
			var calls atomic.Int32
			a := acquisitionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
			}))
			got, err := a.FetchReference(t.Context(), claim)
			if got != nil || !errors.Is(err, tc.want) || calls.Load() != 1 {
				t.Fatal("unexpected status mapping or retry", err, calls.Load())
			}
		})
	}
}

func referenceClaim(t *testing.T, plan problem.ReferencePlan) (problemreference.WorkerClaim, []byte) {
	t.Helper()
	encoding, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	return problemreference.WorkerClaim{RequestID: strings.Repeat("1", 32), SourceID: plan.SourceID,
		PlanSHA256: checksum(encoding), Token: strings.Repeat("a", 64), Until: time.Now().Add(time.Minute)}, encoding
}

func referenceHandler(t *testing.T, claim problemreference.WorkerClaim, encoding []byte, remaining int64, objects map[string][]byte, fail string, calls *atomic.Int32) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input struct {
			problemreference.MaterialInput
			Object materialstore.Object `json:"object"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.RequestID != claim.RequestID || input.SourceID != claim.SourceID || input.PlanSHA256 != claim.PlanSHA256 || input.Token != claim.Token {
			t.Error("incorrect reservation binding")
		}
		switch r.URL.Path {
		case "/api/v1/worker/reference/assignment":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(problemreference.MaterialDescriptor{Encoding: encoding, RemainingMS: remaining})
		case "/api/v1/worker/reference/material":
			raw, ok := objects[input.Object.SHA256]
			if !ok {
				t.Error("unexpected object")
				http.Error(w, "unknown", 404)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("X-Content-SHA256", input.Object.SHA256)
			w.Header().Set("Content-Length", fmt.Sprint(input.Object.Size))
			if fail == "hash" {
				w.Header().Set("X-Content-SHA256", strings.Repeat("f", 64))
			}
			if fail == "length" {
				w.Header().Set("Content-Length", fmt.Sprint(input.Object.Size+1))
			}
			if fail == "corrupt" {
				raw = bytes.Repeat([]byte{'x'}, len(raw))
			}
			if fail == "truncate" && len(raw) > 0 {
				raw = raw[:len(raw)-1]
			}
			_, _ = w.Write(raw)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.Error(w, "unexpected", 404)
		}
	})
}

func TestFetchReferenceFreezesAllBytesAndReleasesQuota(t *testing.T) {
	plan, objects := referenceMaterialsFixture(t)
	claim, encoding := referenceClaim(t, plan)
	var calls atomic.Int32
	a := acquisitionServer(t, referenceHandler(t, claim, encoding, 30000, objects, "", &calls))
	got, err := a.FetchReference(t.Context(), claim)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := got.Close(); err != nil {
			t.Error(err)
		}
	}()
	if !reflect.DeepEqual(got.Plan(), plan) {
		t.Fatal("snapshot plan changed")
	}
	source, err := got.ReadSource(t.Context())
	if err != nil || !bytes.Equal(source, objects[plan.Source.SHA256]) {
		t.Fatal("source mismatch", err)
	}
	for i, c := range plan.Cases {
		input, answer, err := got.ReadCase(t.Context(), i)
		if err != nil || !bytes.Equal(input, objects[c.Input.SHA256]) || !bytes.Equal(answer, objects[c.Answer.SHA256]) {
			t.Fatal("case mismatch", err)
		}
	}
	if second, err := a.FetchReference(t.Context(), claim); err == nil || second != nil {
		t.Fatal("shared snapshot quota bypassed")
	}
	if err := got.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := a.FetchReference(t.Context(), claim)
	if err != nil {
		t.Fatal("close did not release quota", err)
	}
	if err := again.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFetchReferenceRejectsDescriptorBeforeObjectIO(t *testing.T) {
	for _, name := range []string{"digest", "canonical", "source lock", "budget", "zero remaining", "excess remaining"} {
		t.Run(name, func(t *testing.T) {
			plan, objects := referenceMaterialsFixture(t)
			if name == "source lock" {
				plan.SourceLockSHA256 = strings.Repeat("b", 64)
			}
			if name == "budget" {
				plan.Source.Size = problem.SmallReferenceLimits().SourceBytes + 1
			}
			claim, encoding := referenceClaim(t, plan)
			if name == "canonical" {
				encoding = append([]byte(" "), encoding...)
				claim.PlanSHA256 = checksum(encoding)
			}
			if name == "digest" {
				claim.PlanSHA256 = strings.Repeat("b", 64)
			}
			remaining := int64(30000)
			if name == "zero remaining" {
				remaining = 0
			}
			if name == "excess remaining" {
				remaining = 30001
			}
			var calls atomic.Int32
			a := acquisitionServer(t, referenceHandler(t, claim, encoding, remaining, objects, "", &calls))
			got, err := a.FetchReference(t.Context(), claim)
			if got != nil || err == nil || calls.Load() != 1 {
				t.Fatal("invalid descriptor accepted or retried", err, calls.Load())
			}
		})
	}
}

func TestFetchReferenceRejectsObjectFailureAndAllowsExplicitRetry(t *testing.T) {
	for _, name := range []string{"hash", "length", "truncate", "corrupt", "late failure"} {
		t.Run(name, func(t *testing.T) {
			plan, objects := referenceMaterialsFixture(t)
			claim, encoding := referenceClaim(t, plan)
			var calls atomic.Int32
			var failing atomic.Bool
			failing.Store(true)
			base := referenceHandler(t, claim, encoding, 30000, objects, "", &calls)
			broken := referenceHandler(t, claim, encoding, 30000, objects, name, &calls)
			a := acquisitionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if failing.Load() && r.URL.Path == "/api/v1/worker/reference/material" && (name != "late failure" || calls.Load() == 5) {
					if name == "late failure" {
						http.Error(w, "lost", http.StatusServiceUnavailable)
						calls.Add(1)
						return
					}
					broken.ServeHTTP(w, r)
					return
				}
				base.ServeHTTP(w, r)
			}))
			got, err := a.FetchReference(t.Context(), claim)
			wantCalls := int32(2)
			if name == "late failure" {
				wantCalls = 6
			}
			if got != nil || err == nil || calls.Load() != wantCalls {
				t.Fatal("failure accepted or automatically retried", err, calls.Load())
			}
			failing.Store(false)
			again, err := a.FetchReference(t.Context(), claim)
			if err != nil {
				t.Fatal("failure retained quota or prevented explicit retry", err)
			}
			if err := again.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

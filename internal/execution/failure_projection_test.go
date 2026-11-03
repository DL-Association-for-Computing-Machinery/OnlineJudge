package execution

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

type evidenceBranches []error

func (evidenceBranches) Error() string     { panic("error text must not be inspected") }
func (e evidenceBranches) Unwrap() []error { return []error(e) }

type evidenceLink struct{ child error }

func (evidenceLink) Error() string   { panic("error text must not be inspected") }
func (e evidenceLink) Unwrap() error { return e.child }

func TestFailureProjectionSlotBound(t *testing.T) {
	for _, tc := range []struct {
		name      string
		children  int
		truncated bool
	}{
		{"exact-64-including-root", 63, false},
		{"pending-65th-slot", 64, true},
		{"wide-nil", 1 << 18, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := FailureDetails(evidenceBranches(make([]error, tc.children)))
			if d.Truncated != tc.truncated || len(d.Entries) != 1 || d.Entries[0].Code != "unknown" {
				t.Fatalf("slot bound or fallback: %+v", d)
			}
		})
	}
	var deep error = evidenceBranches(make([]error, 1<<18))
	for i := 0; i < 63; i++ {
		deep = evidenceLink{deep}
	}
	if d := FailureDetails(deep); !d.Truncated || len(d.Entries) != 1 {
		t.Fatalf("deep wide branch escaped budget: %+v", d)
	}
	// A 64-slot graph that actually ends has no omitted branch.
	deep = errors.New("fixed")
	for i := 0; i < 63; i++ {
		deep = evidenceLink{deep}
	}
	if d := FailureDetails(deep); d.Truncated || len(d.Entries) != 1 {
		t.Fatalf("exact terminal boundary: %+v", d)
	}
	for _, links := range []int{62, 63} {
		deep = evidenceLink{}
		for i := 0; i < links; i++ {
			deep = evidenceLink{deep}
		}
		if d := FailureDetails(deep); d.Truncated != (links == 63) {
			t.Fatalf("nil single-child slot %d: %+v", links, d)
		}
	}
	// Empty wrappers remain independent unknown causes beside tagged failures.
	d := FailureDetails(errors.Join(evidenceLink{}, evidenceBranches{}, tagFailure(errors.New("fixed"), "delete", "input", "unknown")))
	if d.Truncated || len(d.Entries) != 3 {
		t.Fatalf("empty independent branches lost: %+v", d)
	}
}

func TestFailureProjectionIndependentBranches(t *testing.T) {
	primary := tagFailure(context.Canceled, "control", "stdout", "deadline")
	cleanup := tagFailure(context.DeadlineExceeded, "delete", "input", "deadline")
	unknown := FailureEntry{"unknown", "unknown", "unknown"}
	for _, tc := range []struct {
		err  error
		want []FailureEntry
	}{
		{errors.Join(context.Canceled, cleanup), []FailureEntry{{"unknown", "unknown", "canceled"}, {"delete", "input", "deadline"}}},
		{errors.Join(primary, cleanup, context.DeadlineExceeded), []FailureEntry{{"control", "stdout", "deadline"}, {"delete", "input", "deadline"}, {"unknown", "unknown", "deadline"}}},
		{fmt.Errorf("fixed: %w", errors.Join(errors.New("private"), cleanup)), []FailureEntry{unknown, {"delete", "input", "deadline"}}},
	} {
		d := FailureDetails(tc.err)
		if d.Truncated || !reflect.DeepEqual(d.Entries, tc.want) {
			t.Fatalf("independent cause order: got %+v want %+v", d, tc.want)
		}
	}
	for _, n := range []int{8, 9} {
		branches := make([]error, n)
		for i := range branches {
			branches[i] = cleanup
		}
		d := FailureDetails(errors.Join(branches...))
		if len(d.Entries) != 8 || d.Truncated != (n == 9) {
			t.Fatalf("entry boundary %d: %+v", n, d)
		}
	}
}

func TestCachedFailurePreservesBusyIdentity(t *testing.T) {
	g, _ := evidenceAdapter(t)
	g.busy.Store(true)
	err := g.Verify(context.Background(), g.runtime)
	if !errors.Is(err, ErrBusy) {
		t.Fatal("busy sentinel identity changed")
	}
	d := FailureDetails(err)
	if d.Truncated || !reflect.DeepEqual(d.Entries, []FailureEntry{{"verify", "environment", "ownership"}}) {
		t.Fatalf("busy category: %+v", d)
	}
}

func TestCachedFailureBudgetAndAdmissionCategories(t *testing.T) {
	for _, tc := range []struct{ name, phase, role, code string }{
		{"operation-budget", "verify", "environment", "budget"},
		{"file-budget", "upload", "source", "budget"},
		{"not-verified", "validate", "source", "invalid_evidence"},
		{"foreign-executable", "validate", "executable", "ownership"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := evidenceAdapter(t)
			ctx := context.Background()
			var err error
			switch tc.name {
			case "operation-budget":
				g.operations = g.limits.Operations
				err = g.Verify(ctx, g.runtime)
			case "file-budget":
				if e := g.Verify(ctx, g.runtime); e != nil {
					t.Fatal(e)
				}
				g.limits.FileBytes = 0
				_, err = g.Compile(ctx, g.runtime, "source")
			case "not-verified":
				_, err = g.Compile(ctx, g.runtime, "source")
			case "foreign-executable":
				_, err = g.Run(ctx, g.runtime, "foreign", nil)
			}
			d := FailureDetails(err)
			if d == nil || d.Truncated || !reflect.DeepEqual(d.Entries, []FailureEntry{{tc.phase, tc.role, tc.code}}) {
				t.Fatalf("budget/admission category: %+v", d)
			}
		})
	}
}

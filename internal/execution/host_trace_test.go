package execution

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

type hostEvidenceError struct {
	trace experiment.HostFailureTrace
	cause error
}

func (*hostEvidenceError) Error() string   { panic("private host error text must not be read") }
func (e *hostEvidenceError) Unwrap() error { return e.cause }
func (e *hostEvidenceError) SandboxHostTrace() (experiment.HostFailureTrace, bool) {
	return e.trace, true
}

func hostEvidence(phase int, elapsed int64) *hostEvidenceError {
	phases := [...]string{"admission", "prepare", "inspect", "command", "response_validation"}
	raw := experiment.HostFailureTrace{Phase: phases[phase], LastCompleted: "none", Count: phase + 1, ElapsedMS: elapsed}
	if phase > 0 {
		raw.LastCompleted = phases[phase-1]
	}
	for i := 0; i <= phase; i++ {
		raw.Steps[i] = experiment.HostFailureStep{Phase: phases[i], Completed: i < phase, ElapsedMS: 1, ContextBefore: "active", HasDeadline: true, RemainingMS: 1000}
		if i == 2 || i == 3 {
			raw.Steps[i].CallAttempted = true
			raw.Steps[i].LocalProcessStart = "unknown"
		}
	}
	return &hostEvidenceError{trace: raw, cause: context.DeadlineExceeded}
}

func TestHostTraceProjectionBoundsAndDetachedValues(t *testing.T) {
	original := hostEvidence(3, 60001)
	original.trace.Steps[0].ElapsedMS = -1
	original.trace.Steps[0].CallAttempted = true
	original.trace.Steps[0].LocalProcessStart = "started"
	original.trace.Steps[1].RemainingMS = 1 << 62
	original.trace.Steps[2].HasDeadline = false
	original.trace.Steps[2].RemainingMS = 123
	original.trace.Steps[2].RemainingCapped = true
	original.trace.Steps[3].ContextBefore = "PRIVATE-context"
	original.trace.Steps[3].LocalProcessStart = "PRIVATE-process"
	original.trace.Steps[4].Phase = "PRIVATE-unreached"
	err := withFailureMetadata(errors.New("fixed"), original)
	if errors.Is(err, context.DeadlineExceeded) || errorFailureCode(err) != "deadline" {
		t.Fatal("metadata reconnected an old discarded cause or changed category")
	}
	raw, present := err.(hostTraceProvider).SandboxHostTrace()
	if !present || raw.Steps[4] != (experiment.HostFailureStep{}) {
		t.Fatal("inactive array slots retained evidence")
	}
	tagged := tagFailure(err, "control", "stdout", "")
	d := FailureDetails(tagged)
	h := d.Entries[0].HostTrace
	if h == nil || len(h.Steps) != 4 || h.ElapsedMS != 60000 || !h.ElapsedCapped || h.Steps[0].ElapsedMS != 0 || !h.Steps[0].ElapsedCapped || h.Steps[0].CallAttempted || h.Steps[0].LocalProcessStart != "" || h.Steps[1].RemainingMS != 60000 || !h.Steps[1].RemainingCapped || h.Steps[2].RemainingMS != 0 || h.Steps[2].RemainingCapped || h.Steps[3].ContextBefore != "unknown" || h.Steps[3].LocalProcessStart != "unknown" {
		t.Fatal("trace projection escaped numeric or enum bounds")
	}
	encoded, e := json.Marshal(d)
	if e != nil || strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("private evidence escaped projection")
	}
	h.Steps[0].Phase = "changed"
	original.trace.Phase = "PRIVATE-mutated-after-tag"
	if FailureDetails(tagged).Entries[0].HostTrace.Steps[0].Phase != "admission" {
		t.Fatal("projection or tagged metadata aliases its caller")
	}
	retained, ok := hostTraceFrom(tagged)
	if !ok || retained.Phase != "command" {
		t.Fatal("later error mutation replaced frozen tagged evidence")
	}
}

func TestHostTraceRejectsMalformedStructureAndAmbiguousBranches(t *testing.T) {
	for _, mode := range []string{"zero", "negative", "large", "phase", "last", "step", "completion"} {
		t.Run(mode, func(t *testing.T) {
			e := hostEvidence(3, 1)
			switch mode {
			case "zero":
				e.trace.Count = 0
			case "negative":
				e.trace.Count = -1
			case "large":
				e.trace.Count = 1 << 30
			case "phase":
				e.trace.Phase = "PRIVATE-phase"
			case "last":
				e.trace.LastCompleted = "PRIVATE-last"
			case "step":
				e.trace.Steps[1].Phase = "command"
			case "completion":
				e.trace.Steps[3].Completed = true
			}
			if hostTraceDetails(e) != nil || FailureDetails(tagFailure(e, "control", "stdout", "")).Entries[0].HostTrace != nil {
				t.Fatal("malformed structure admitted")
			}
		})
	}
	a, b := hostEvidence(2, 10), hostEvidence(3, 20)
	for _, err := range []error{errors.Join(a, b), errors.Join(errors.New("private"), a), errors.Join(tagFailure(errors.New("no trace"), "delete", "input", "unknown"), a)} {
		if value, present := hostTraceFrom(err); present || value != (experiment.HostFailureTrace{}) {
			t.Fatal("one operation borrowed evidence from an independent branch")
		}
	}
	if h := hostTraceDetails(errors.Join(a, context.Canceled)); h == nil || h.Phase != "inspect" {
		t.Fatal("context annotation concealed the same operation")
	}
	for _, links := range []int{63, 64} {
		var deep error = a
		for i := 0; i < links; i++ {
			deep = evidenceLink{deep}
		}
		if _, present := hostTraceFrom(deep); present != (links == 63) {
			t.Fatal("metadata traversal changed the 64-slot bound")
		}
	}
}

type hostEvidenceTransport struct {
	*evidenceFiles
	runError error
}

func (f *hostEvidenceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == "/run" && f.compiles > 0 && f.runError != nil {
		return nil, f.runError
	}
	return f.evidenceFiles.RoundTrip(r)
}

func TestHostTraceSurvivesControlAndIndependentCachedCleanup(t *testing.T) {
	f := &hostEvidenceTransport{evidenceFiles: &evidenceFiles{cacheFixture: &cacheFixture{t: t, data: map[string][]byte{}}, roles: map[string]string{}, deletes: map[string]error{}}}
	g := NewCachedGoJudge(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f)
	ctx, p := context.Background(), experiment.PinnedEnvironment()
	if err := g.Verify(ctx, p); err != nil {
		t.Fatal(err)
	}
	o, err := g.Compile(ctx, p, "source")
	if err != nil {
		t.Fatal(err)
	}
	f.runError = hostEvidence(3, 10)
	f.deletes["input"] = hostEvidence(2, 20)
	f.deletes["executable"] = hostEvidence(4, 30)
	_, runErr := g.Run(ctx, p, o.Artifact, []byte("input"))
	cleanup := g.Remove(ctx, o.Artifact)
	d := FailureDetails(errors.Join(runErr, cleanup, context.Canceled))
	if len(d.Entries) != 4 || d.Truncated || d.Entries[0].Phase != "control" || d.Entries[0].Role != "stdout" || d.Entries[1].Role != "input" || d.Entries[2].Role != "executable" || d.Entries[3].Code != "canceled" || d.Entries[3].HostTrace != nil {
		t.Fatal("independent error order or association changed")
	}
	for i, phase := range []string{"command", "inspect", "response_validation"} {
		h := d.Entries[i].HostTrace
		if h == nil || h.Phase != phase || h.ElapsedMS != int64((i+1)*10) || d.Entries[i].Code != "deadline" {
			t.Fatal("trace lost or borrowed across cleanup", i)
		}
	}
	if errors.Is(runErr, context.DeadlineExceeded) || errors.Is(cleanup, context.DeadlineExceeded) {
		t.Fatal("control metadata changed old sanitized context semantics")
	}
	// The cached helper retains only the context cause it already retained.
	e := hostEvidence(3, 5)
	cached := cachedError(ctx, "fixed", e)
	if !errors.Is(cached, context.DeadlineExceeded) || hostTraceDetails(cached) == nil {
		t.Fatal("cached cause or copied trace lost")
	}
}

func TestHostTraceCopiesAndEightEntryBound(t *testing.T) {
	err := tagFailure(hostEvidence(3, 7), "control", "stdout", "deadline")
	for _, count := range []int{8, 9} {
		branches := make([]error, count)
		for i := range branches {
			branches[i] = err
		}
		d := FailureDetails(errors.Join(branches...))
		if len(d.Entries) != 8 || d.Truncated != (count == 9) {
			t.Fatal("trace widened failure entry bounds")
		}
		d.Entries[0].HostTrace.Steps[0].Phase = "changed"
		if d.Entries[1].HostTrace.Steps[0].Phase != "admission" {
			t.Fatal("two projected operations share a trace")
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := FailureDetails(err)
			d.Entries[0].HostTrace.Steps[0].Phase = "changed"
			if fresh := FailureDetails(err).Entries[0].HostTrace; fresh.Steps[0].Phase != "admission" {
				t.Error("concurrent projection aliases retained evidence")
			}
		}()
	}
	wg.Wait()
	h := FailureDetails(err).Entries[0].HostTrace
	clone := CloneHostTrace(h)
	if !reflect.DeepEqual(h, clone) || h == clone {
		t.Fatal("report clone not detached")
	}
	clone.Steps = append(clone.Steps, clone.Steps[0], clone.Steps[0])
	if CloneHostTrace(clone) != nil {
		t.Fatal("overlong public trace admitted")
	}
}

type countedHostProvider struct {
	hostEvidenceError
	unwraps int
}

func (e *countedHostProvider) Unwrap() error { e.unwraps++; return e }

type countedHostLink struct {
	child  error
	visits *int
}

func (*countedHostLink) Error() string   { panic("private wrapper must not be formatted") }
func (e *countedHostLink) Unwrap() error { *e.visits = *e.visits + 1; return e.child }

func TestHostTraceProjectionKeepsOneSharedTraversalBudget(t *testing.T) {
	var branches []error
	var providers []*countedHostProvider
	for i := 0; i < 8; i++ {
		e := &countedHostProvider{hostEvidenceError: *hostEvidence(3, 1)}
		providers = append(providers, e)
		branches = append(branches, e)
	}
	d := FailureDetails(errors.Join(branches...))
	if d.Truncated || len(d.Entries) != 8 {
		t.Fatal("eight terminal providers changed projection bounds")
	}
	for i, e := range providers {
		if e.unwraps != 0 || d.Entries[i].Code != "unknown" || d.Entries[i].HostTrace == nil {
			t.Fatal("terminal provider started a second category traversal")
		}
	}
	for _, links := range []int{63, 64} {
		visits := 0
		provider := &countedHostProvider{hostEvidenceError: *hostEvidence(3, 1)}
		var deep error = provider
		for i := 0; i < links; i++ {
			deep = &countedHostLink{child: deep, visits: &visits}
		}
		d := FailureDetails(deep)
		if visits != links || provider.unwraps != 0 || d.Truncated != (links == 64) || (d.Entries[0].HostTrace != nil) != (links == 63) {
			t.Fatal("deep provider escaped the single 64-slot traversal")
		}
	}
}

package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

type evidenceFiles struct {
	*cacheFixture
	mode    string
	roles   map[string]string
	deletes map[string]error
	cancel  context.CancelFunc
}

func (f *evidenceFiles) UploadFile(ctx context.Context, data []byte) (string, error) {
	role := "source"
	if f.compiles > 0 {
		role = "input"
	}
	if f.mode == role+"-upload" {
		return "", errors.Join(errors.New("PRIVATE-source-and-path"), context.Canceled)
	}
	id, err := f.cacheFixture.UploadFile(ctx, data)
	f.roles[id] = role
	return id, err
}

func (f *evidenceFiles) ReadFile(ctx context.Context, id string, maximum int64) ([]byte, error) {
	role := f.roles[id]
	if f.mode == role+"-read" {
		return nil, errors.New("PRIVATE-input-and-command")
	}
	if f.mode == role+"-integrity" {
		if f.cancel != nil {
			f.cancel()
		}
		raw := append([]byte(nil), f.data[id]...)
		if len(raw) > 0 {
			raw[0] ^= 255
		}
		return raw, nil
	}
	return f.cacheFixture.ReadFile(ctx, id, maximum)
}

func (f *evidenceFiles) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == "/version" && f.mode == "verify" {
		return nil, errors.New("PRIVATE-inspection")
	}
	if r.Method == "DELETE" {
		id := strings.TrimPrefix(r.URL.Path, "/file/")
		if err := f.deletes[f.roles[id]]; err != nil {
			return nil, err
		}
	}
	if r.URL.Path == "/run" {
		op := "compile"
		if f.compiles > 0 {
			op = "run"
		}
		if f.mode == op+"-control" {
			return nil, fmt.Errorf("PRIVATE-transport: %w", context.DeadlineExceeded)
		}
		if f.mode == op+"-decode" {
			return response("invalid JSON"), nil
		}
	}
	compiles, runs := f.compiles, f.runs
	response, err := f.cacheFixture.RoundTrip(r)
	if f.compiles > compiles {
		f.roles["object-"+strconv.Itoa(f.next)] = "executable"
	} else if f.runs > runs {
		f.roles["object-"+strconv.Itoa(f.next)] = "stdout"
	}
	return response, err
}

func evidenceAdapter(t *testing.T) (*CachedGoJudge, *evidenceFiles) {
	t.Helper()
	f := &evidenceFiles{cacheFixture: &cacheFixture{t: t, data: map[string][]byte{}}, roles: map[string]string{}, deletes: map[string]error{}}
	g := NewCachedGoJudge(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f)
	return g, f
}

func TestCachedFailurePhasesRolesAndSecrecy(t *testing.T) {
	for _, tc := range []struct{ mode, phase, role, code string }{
		{"verify", "verify", "environment", "unknown"},
		{"source-upload", "upload", "source", "canceled"},
		{"source-read", "readback", "source", "unknown"},
		{"source-integrity", "readback", "source", "integrity"},
		{"compile-control", "control", "executable", "deadline"},
		{"compile-decode", "control", "executable", "decode"},
		{"source-delete", "delete", "source", "unknown"},
		{"input-upload", "upload", "input", "canceled"},
		{"input-read", "readback", "input", "unknown"},
		{"input-integrity", "readback", "input", "integrity"},
		{"run-control", "control", "stdout", "deadline"},
		{"run-decode", "control", "stdout", "decode"},
		{"stdout-read", "stdout_read", "stdout", "unknown"},
		{"stdout-delete", "delete", "stdout", "unknown"},
		{"input-delete", "delete", "input", "unknown"},
		{"executable-delete", "delete", "executable", "unknown"},
		{"negative", "validate", "stdout", "invalid_evidence"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			ctx, p := context.Background(), experiment.PinnedEnvironment()
			g, f := evidenceAdapter(t)
			if tc.mode == "verify" {
				f.mode = tc.mode
			}
			err := g.Verify(ctx, p)
			if err == nil {
				if strings.HasPrefix(tc.mode, "source-") || strings.HasPrefix(tc.mode, "compile-") {
					f.mode = tc.mode
				}
				if tc.mode == "source-delete" {
					f.deletes["source"] = errors.New("PRIVATE-delete")
				}
				var compiled Observation
				compiled, err = g.Compile(ctx, p, "source")
				if err == nil {
					f.mode = tc.mode
					if tc.mode == "negative" {
						f.fail = "negative"
					}
					if strings.HasSuffix(tc.mode, "-delete") {
						f.deletes[tc.role] = errors.New("PRIVATE-delete")
					}
					if tc.role == "executable" && tc.phase == "delete" {
						err = g.Remove(ctx, compiled.Artifact)
					} else {
						_, err = g.Run(ctx, p, compiled.Artifact, []byte("input"))
					}
				}
			}
			d := FailureDetails(err)
			if d == nil || len(d.Entries) != 1 || d.Entries[0] != (FailureEntry{Phase: tc.phase, Role: tc.role, Code: tc.code}) {
				t.Fatalf("failure evidence=%+v", d)
			}
			raw, e := json.Marshal(d)
			if e != nil || strings.Contains(string(raw)+err.Error(), "PRIVATE") {
				t.Fatal("raw failure leaked")
			}
			walkFailures(err, func(node error) bool {
				if strings.Contains(node.Error(), "PRIVATE") {
					t.Fatal("raw failure retained in error chain")
				}
				return true
			})
			if strings.HasSuffix(tc.mode, "-upload") && !errors.Is(err, context.Canceled) {
				t.Fatal("existing file context identity lost")
			}
			if strings.HasSuffix(tc.mode, "-control") && errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("control metadata changed old context unwrapping")
			}
		})
	}
}

func TestJoinedPrimaryAndCleanupEvidenceKeepsContext(t *testing.T) {
	g, f := evidenceAdapter(t)
	p := experiment.PinnedEnvironment()
	if err := g.Verify(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	compiled, err := g.Compile(context.Background(), p, "source")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.mode, f.cancel = "input-integrity", cancel
	f.deletes["input"] = context.DeadlineExceeded
	f.deletes["executable"] = errors.New("PRIVATE-executable-delete")
	_, primary := g.Run(ctx, p, compiled.Artifact, []byte("input"))
	cleanup := g.Remove(context.Background(), compiled.Artifact)
	err = errors.Join(primary, cleanup)
	d := FailureDetails(err)
	want := []FailureEntry{{"readback", "input", "integrity"}, {"delete", "input", "deadline"}, {"unknown", "unknown", "canceled"}, {"delete", "executable", "unknown"}}
	if d == nil || fmt.Sprint(d.Entries) != fmt.Sprint(want) || d.Truncated || !errors.Is(err, context.Canceled) {
		t.Fatalf("joined failure lost cause or order: %+v", d)
	}
}

type cyclicEvidenceError struct{ visits int }

func (*cyclicEvidenceError) Error() string   { panic("projection must not inspect error text") }
func (e *cyclicEvidenceError) Unwrap() error { e.visits++; return e }

func TestFailureProjectionBoundsCopiesAndUnknownText(t *testing.T) {
	cycle := &cyclicEvidenceError{}
	d := FailureDetails(cycle)
	if !d.Truncated || cycle.visits != 64 || d.Entries[0] != (FailureEntry{"unknown", "unknown", "unknown"}) {
		t.Fatal("unbounded error graph traversal")
	}
	var failures []error
	for i := 0; i < 12; i++ {
		failures = append(failures, tagFailure(errors.New("fixed"), "PRIVATE", "PRIVATE", "PRIVATE"))
	}
	err := errors.Join(failures...)
	d = FailureDetails(err)
	if !d.Truncated || len(d.Entries) != 8 || d.Entries[0] != (FailureEntry{"unknown", "unknown", "unknown"}) {
		t.Fatal("entry cap or whitelist bypassed")
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Go(func() {
			copy := FailureDetails(err)
			copy.Entries[0].Code = "PRIVATE"
			if FailureDetails(err).Entries[0].Code != "unknown" {
				t.Error("projection shares mutable state")
			}
		})
	}
	wg.Wait()
	if FailureDetails(nil) != nil {
		t.Fatal("success has failure metadata")
	}
}

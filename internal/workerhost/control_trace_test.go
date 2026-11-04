package workerhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func controlDetails(t *testing.T, err error) experiment.HostFailureTrace {
	t.Helper()
	var provider interface {
		SandboxHostTrace() (experiment.HostFailureTrace, bool)
	}
	if err == nil || !errors.As(err, &provider) {
		t.Fatal("missing control trace")
	}
	d, ok := provider.SandboxHostTrace()
	if !ok {
		t.Fatal("unavailable control trace")
	}
	return d
}

func TestControlTraceReachedStagesAndDetachedValues(t *testing.T) {
	phases := []string{"admission", "prepare", "inspect", "command", "response_validation"}
	for index, phase := range phases {
		t.Run(phase, func(t *testing.T) {
			h, f := opened(t)
			before := len(f.calls)
			req, err := http.NewRequest("POST", "http://127.0.0.1:5050/run", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			sentinel := errors.New("PRIVATE-cause")
			switch phase {
			case "admission":
				h.closed = true
			case "prepare":
				req.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", requestLimit+1)))
			case "inspect":
				h.run = func(context.Context, io.Reader, string, ...string) ([]byte, error) {
					return nil, &commandFailure{code: "unknown", cause: sentinel, processStart: "not_started"}
				}
			case "command":
				h.run = func(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
					if args[2] == "inspect" {
						return f.run(ctx, input, name, args...)
					}
					return nil, sentinel
				}
			case "response_validation":
				f.response = "PRIVATE-body\n500"
			}
			_, err = h.RoundTrip(req)
			d := controlDetails(t, err)
			last := "none"
			if index != 0 {
				last = phases[index-1]
			}
			if d.Phase != phase || d.Count != index+1 || d.LastCompleted != last {
				t.Fatal("wrong failure position or reached stages")
			}
			for i := 0; i < d.Count; i++ {
				step := d.Steps[i]
				if step.Phase != phases[i] || step.Completed != (i < index) || step.ElapsedMS < 0 || step.ElapsedMS > 60000 {
					t.Fatal("step ordering/completion or elapsed bounds")
				}
				if step.LocalProcessStart == "started" {
					t.Fatal("test double or nil return established process startup")
				}
			}
			if index < 2 && len(f.calls) != before || (phase == "inspect" || phase == "command") && !errors.Is(err, sentinel) {
				t.Fatal("early failure dispatched IO or changed original cause")
			}
			encoded, e := json.Marshal(d)
			if e != nil || strings.Contains(string(encoded), "PRIVATE") || strings.Contains(string(encoded), h.id) {
				t.Fatal("trace retained external body/cause/identity")
			}
			d.Steps[0].Phase = "PRIVATE"
			if controlDetails(t, err).Steps[0].Phase != "admission" {
				t.Fatal("returned value aliases retained trace")
			}
		})
	}
}

func TestControlTraceCancellationKeepsCommandAttemptOrder(t *testing.T) {
	h, f := opened(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	commandCalls := 0
	h.run = func(ctx context.Context, _ io.Reader, _ string, args ...string) ([]byte, error) {
		if args[2] == "inspect" {
			out, err := json.Marshal(f.s)
			cancel()
			return out, err
		}
		commandCalls++
		return runCommand(ctx, nil, "/bin/sh", "-c", "exit 0")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:5050/run", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.RoundTrip(req)
	d := controlDetails(t, err)
	if !errors.Is(err, context.Canceled) || commandCalls != 1 || d.Phase != "command" || d.LastCompleted != "inspect" || d.Steps[3].ContextBefore != "canceled" || !d.Steps[3].CallAttempted || d.Steps[3].LocalProcessStart != "not_started" {
		t.Fatal("instrumentation changed expired-context command attempt semantics")
	}
	_, err = h.RoundTrip(req)
	d = controlDetails(t, err)
	if d.Count != 1 || d.Phase != "admission" || d.Steps[0].ContextBefore != "canceled" || commandCalls != 1 {
		t.Fatal("already canceled admission reached inspect or command")
	}
}

func TestControlTraceRealLocalProcessStartup(t *testing.T) {
	h, f := opened(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	signal := &commandStartedSignal{started: make(chan struct{})}
	h.run = func(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
		if args[2] == "inspect" {
			return f.run(ctx, input, name, args...)
		}
		return nil, runCommandTo(ctx, input, signal, 16, "/bin/sh", "-c", "printf ready; exec /bin/sleep 30")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:5050/run", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.RoundTrip(req)
	d := controlDetails(t, err)
	select {
	case <-signal.started:
	default:
		t.Fatal("trusted local command did not start")
	}
	if !errors.Is(err, context.DeadlineExceeded) || d.Phase != "command" || d.Steps[3].ContextBefore != "active" || d.Steps[3].LocalProcessStart != "started" {
		t.Fatal("actual local process startup evidence lost")
	}
}

func TestControlTraceDurationAndDeadlineBounds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	trace := newControlTrace(ctx)
	trace.started, trace.stepStarted = time.Now().Add(-2*time.Minute), time.Now().Add(-2*time.Minute)
	d := controlDetails(t, trace.attach(errors.New("fixed")))
	if d.ElapsedMS != 60000 || !d.ElapsedCapped || d.Steps[0].ElapsedMS != 60000 || !d.Steps[0].ElapsedCapped || !d.Steps[0].HasDeadline || d.Steps[0].RemainingMS != 60000 || !d.Steps[0].RemainingCapped {
		t.Fatal("control timing/deadline not capped")
	}
	if newControlTrace(context.Background()).detail.Steps[0].HasDeadline || trace.attach(nil) != nil {
		t.Fatal("no-deadline or success semantics changed")
	}
	expired, expiredDone := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer expiredDone()
	step := newControlTrace(expired).detail.Steps[0]
	if step.ContextBefore != "deadline" || !step.HasDeadline || step.RemainingMS != 0 || !step.RemainingCapped {
		t.Fatal("expired deadline evidence not distinguished from rounded zero")
	}
}

type controlClosingBody struct{ io.Reader }

func (controlClosingBody) Close() error {
	timer := time.NewTimer(25 * time.Millisecond)
	defer timer.Stop()
	<-timer.C
	return nil
}

func TestControlTraceFreezesFailureBeforeDeferredCleanup(t *testing.T) {
	h, f := opened(t)
	f.response = "{}\n500"
	req, err := http.NewRequest("POST", "http://127.0.0.1:5050/run", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Body = controlClosingBody{strings.NewReader("{}")}
	_, err = h.RoundTrip(req)
	d := controlDetails(t, err)
	var stages int64
	for i := 0; i < d.Count; i++ {
		stages += d.Steps[i].ElapsedMS
	}
	if d.Phase != "response_validation" || d.ElapsedMS-stages < 20 {
		t.Fatal("failed phase absorbed deferred cleanup time")
	}
}

func TestControlTraceInspectValidationAndWrappedProcessEvidence(t *testing.T) {
	for _, mode := range []string{"decode", "isolation", "wrapped", "success"} {
		t.Run(mode, func(t *testing.T) {
			h, f := opened(t)
			commands := 0
			h.run = func(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
				if args[2] == "inspect" {
					if mode == "decode" {
						return []byte("{"), nil
					}
					if mode == "isolation" {
						f.s.Running = false
					}
					return f.run(ctx, input, name, args...)
				}
				commands++
				if mode == "wrapped" {
					return nil, fmt.Errorf("fixed: %w", &commandFailure{code: "deadline", processStart: "started"})
				}
				return f.run(ctx, input, name, args...)
			}
			req, err := http.NewRequest("POST", "http://127.0.0.1:5050/run", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := h.RoundTrip(req)
			if mode == "success" {
				if err != nil || response == nil || commands != 1 {
					t.Fatal("successful control operation acquired a failure")
				}
				if err := response.Body.Close(); err != nil {
					t.Fatal(err)
				}
				return
			}
			d := controlDetails(t, err)
			if mode == "wrapped" {
				if commands != 1 || d.Phase != "command" || d.Steps[3].LocalProcessStart != "unknown" {
					t.Fatal("wrapped process failure established direct startup evidence")
				}
			} else if commands != 0 || d.Phase != "inspect" || d.LastCompleted != "prepare" || d.Steps[2].LocalProcessStart != "unknown" {
				t.Fatal("invalid inspection reached command or acquired process evidence")
			}
		})
	}
}

func TestHostTraceCrossesExecutionTransportWithoutOldCause(t *testing.T) {
	h, f := opened(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.run = func(ctx context.Context, _ io.Reader, _ string, args ...string) ([]byte, error) {
		if args[2] == "inspect" {
			out, err := json.Marshal(f.s)
			cancel()
			return out, err
		}
		return runCommand(ctx, nil, "/bin/sh", "-c", "exit 0")
	}
	_, err := execution.NewGoJudge(h, nil).Compile(ctx, experiment.PinnedEnvironment(), "PRIVATE-source")
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatal("execution transport changed its old sanitized error chain")
	}
	d := execution.FailureDetails(err)
	if d == nil || len(d.Entries) != 1 || d.Entries[0].Code != "canceled" || d.Entries[0].HostTrace == nil || d.Entries[0].HostTrace.Phase != "command" || d.Entries[0].HostTrace.Steps[3].LocalProcessStart != "not_started" {
		t.Fatal("actual Host typed metadata did not cross execution transport")
	}
	encoded, err := json.Marshal(d)
	if err != nil || strings.Contains(string(encoded), "PRIVATE") || strings.Contains(string(encoded), h.id) {
		t.Fatal("cross-boundary metadata leaked command identity or source")
	}
}

func TestSuccessfulHostThenDecodeFailureHasNoHostTrace(t *testing.T) {
	h, f := opened(t)
	f.response = "invalid JSON\n200"
	_, err := execution.NewGoJudge(h, nil).Compile(context.Background(), experiment.PinnedEnvironment(), "trusted fixture")
	var syntax *json.SyntaxError
	d := execution.FailureDetails(err)
	if !errors.As(err, &syntax) || d == nil || len(d.Entries) != 1 || d.Entries[0].Code != "decode" || d.Entries[0].HostTrace != nil {
		t.Fatal("success-side metadata was invented or decode cause changed")
	}
}

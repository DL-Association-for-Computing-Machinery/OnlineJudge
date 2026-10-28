package workerhost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func goodState() state {
	p := experiment.PinnedEnvironment()
	return state{ID: strings.Repeat("a", 64), Started: "2026-10-28T00:00:00Z", Image: p.ImageID,
		Running: true, PID: 42, Network: "none", Readonly: true, Privileged: true,
		Memory: 1 << 30, CPUs: 1e9, Pids: 128, Shm: 128 << 20, Restart: "no", IPCMode: "private",
		Entrypoint: p.SandboxArgs, Tmpfs: map[string]string{"/tmp": "rw,nosuid,size=128m"}}
}

type fakeHost struct {
	mu       sync.Mutex
	s        state
	calls    [][]string
	response string
	fail     string
}

func (f *fakeHost) run(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	f.calls = append(f.calls, append([]string{name}, args...))
	if name == "colima" {
		if f.fail == "pid" {
			return nil, errors.New("still present")
		}
		return nil, nil
	}
	if len(args) < 3 || !reflect.DeepEqual(args[:2], []string{"--context", "colima-oj-m0"}) {
		return nil, errors.New("wrong context")
	}
	switch args[2] {
	case "inspect":
		return json.Marshal(f.s)
	case "image":
		return []byte(experiment.PinnedEnvironment().Platform), nil
	case "exec":
		if args[3] == "-i" {
			return []byte(f.response), nil
		}
		if args[4] == "uname" {
			return []byte(experiment.PinnedEnvironment().KernelRelease), nil
		}
		return []byte(experiment.PinnedEnvironment().CompilerVersion), nil
	case "top":
		if f.fail == "top" {
			return nil, errors.New("unavailable")
		}
		return []byte("PID\n42\n43\n"), nil
	case "kill":
		if f.fail == "kill" {
			return nil, errors.New("unknown")
		}
		f.s.Running, f.s.PID = false, 0
		return nil, nil
	}
	return nil, errors.New("unexpected command")
}

func opened(t *testing.T) (*Host, *fakeHost) {
	t.Helper()
	f := &fakeHost{s: goodState(), response: "{}\n200"}
	h, err := open(context.Background(), f.run)
	if err != nil {
		t.Fatal(err)
	}
	return h, f
}

func TestHostRejectsDrift(t *testing.T) {
	for name, change := range map[string]func(*state){
		"id": func(s *state) { s.ID = "short" }, "image": func(s *state) { s.Image = "other" },
		"network": func(s *state) { s.Network = "host" }, "mount": func(s *state) { s.Mounts = 1 },
		"writable": func(s *state) { s.Readonly = false }, "memory": func(s *state) { s.Memory++ },
		"cpu": func(s *state) { s.CPUs++ }, "pids": func(s *state) { s.Pids = 0 },
		"restart": func(s *state) { s.Restart = "always" }, "pid namespace": func(s *state) { s.PIDMode = "host" },
		"ipc": func(s *state) { s.IPCMode = "host" }, "entrypoint": func(s *state) { s.Entrypoint = []string{"sh"} },
		"tmpfs": func(s *state) { s.Tmpfs = nil }, "shm": func(s *state) { s.Shm++ },
		"paused": func(s *state) { s.Paused = true }, "stopped": func(s *state) { s.Running = false },
		"privilege": func(s *state) { s.Privileged = false }, "cmd": func(s *state) { s.Cmd = []string{"extra"} },
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeHost{s: goodState()}
			change(&f.s)
			if _, err := open(context.Background(), f.run); err == nil {
				t.Fatal("accepted incompatible host")
			}
		})
	}
	h, f := opened(t)
	f.s.Started = "new lifetime"
	if err := h.Verify(context.Background(), experiment.PinnedEnvironment()); err == nil {
		t.Fatal("accepted restart")
	}
	if err := h.Stop(context.Background()); err == nil {
		t.Fatal("stopped different lifetime")
	}
	for _, c := range f.calls {
		if len(c) > 3 && c[3] == "kill" {
			t.Fatal("killed changed lifetime")
		}
	}
}

func TestTransportBoundary(t *testing.T) {
	for _, raw := range []string{"https://127.0.0.1:5050/run", "http://other/run", "http://127.0.0.1:5050/run?x=1", "http://127.0.0.1:5050/run#x", "http://user@127.0.0.1:5050/run", "http://127.0.0.1:5050/file/../../x", "http://127.0.0.1:5050/%72un"} {
		h, f := opened(t)
		before := len(f.calls)
		r, _ := http.NewRequest("POST", raw, nil)
		if _, err := h.RoundTrip(r); err == nil || len(f.calls) != before {
			t.Fatalf("dispatched %s", raw)
		}
	}
	for _, response := range []string{"body", "{}\n301", "{}\n500", "{}\n000", "{}\n200junk"} {
		h, f := opened(t)
		f.response = response
		r, _ := http.NewRequest("POST", "http://127.0.0.1:5050/run", strings.NewReader("{}"))
		if _, err := h.RoundTrip(r); err == nil {
			t.Fatalf("accepted %q", response)
		}
	}
	h, f := opened(t)
	r, _ := http.NewRequest("POST", "http://127.0.0.1:5050/run", strings.NewReader(strings.Repeat("a", requestLimit+1)))
	before := len(f.calls)
	if _, err := h.RoundTrip(r); err == nil || len(f.calls) != before {
		t.Fatal("oversized request dispatched")
	}
	r, _ = http.NewRequest("POST", "http://127.0.0.1:5050/run", strings.NewReader("{}"))
	resp, err := h.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "{}" || resp.StatusCode != 200 {
		t.Fatal("invalid response")
	}
	call := strings.Join(f.calls[len(f.calls)-1], " ")
	if !strings.Contains(call, "exec -i "+h.id+" curl --disable") || !strings.Contains(call, "--noproxy *") {
		t.Fatal("missing pinned transport controls")
	}
	escaped, _ := json.Marshal(map[string]string{"source": strings.Repeat("<", 64*1024)})
	r, _ = http.NewRequest("POST", "http://127.0.0.1:5050/run", strings.NewReader(string(escaped)))
	resp, err = h.RoundTrip(r)
	if err != nil {
		t.Fatalf("legal escaped source rejected: %v", err)
	}
	resp.Body.Close()
	f.s.Started = "new lifetime"
	r, _ = http.NewRequest("DELETE", "http://127.0.0.1:5050/file/abc", nil)
	if _, err := h.RoundTrip(r); err == nil {
		t.Fatal("dispatch after external restart")
	}
}

func TestStopEvidenceAndFence(t *testing.T) {
	for _, failure := range []string{"", "top", "kill", "pid"} {
		t.Run(failure, func(t *testing.T) {
			h, f := opened(t)
			f.fail = failure
			err := h.Stop(context.Background())
			if (err == nil) != (failure == "") {
				t.Fatalf("stop: %v", err)
			}
			before := len(f.calls)
			if err := h.Verify(context.Background(), experiment.PinnedEnvironment()); err == nil {
				t.Fatal("verify after stop")
			}
			r, _ := http.NewRequest("GET", "http://127.0.0.1:5050/version", nil)
			if _, err := h.RoundTrip(r); err == nil {
				t.Fatal("request after stop")
			}
			if len(f.calls) != before {
				t.Fatal("command after fence")
			}
			killed := false
			for _, c := range f.calls {
				if len(c) > 3 && c[3] == "kill" {
					killed = true
				}
			}
			if !killed {
				t.Fatal("failed to attempt kill")
			}
		})
	}
}

func TestStopCancelsAndDrainsDispatch(t *testing.T) {
	h, f := opened(t)
	entered := make(chan struct{})
	drained := make(chan struct{})
	h.run = func(ctx context.Context, in io.Reader, name string, args ...string) ([]byte, error) {
		if len(args) > 3 && args[2] == "exec" && args[3] == "-i" {
			close(entered)
			<-ctx.Done()
			close(drained)
			return nil, ctx.Err()
		}
		if len(args) > 2 && args[2] == "kill" {
			select {
			case <-drained:
			default:
				t.Error("kill before client drain")
			}
		}
		return f.run(ctx, in, name, args...)
	}
	finished := make(chan error, 1)
	go func() {
		r, _ := http.NewRequest("POST", "http://127.0.0.1:5050/run", strings.NewReader("{}"))
		_, err := h.RoundTrip(r)
		finished <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request not dispatched")
	}
	if err := h.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("request not cancelled: %v", err)
	}
}

func TestCommandBudgetsAndPIDParsing(t *testing.T) {
	var b limitedBuffer
	if _, err := b.Write(make([]byte, 128*1024+2)); err == nil || b.Len() != 0 {
		t.Fatal("unbounded output")
	}
	for _, s := range []string{"", "PID", "PID\n1", "PID\n43", "PID\n42\n42", "PID\n42\nno"} {
		if _, err := parsePIDs(s, 42); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runCommand(ctx, nil, "does-not-exist"); err == nil {
		t.Fatal("cancelled command succeeded")
	}
}

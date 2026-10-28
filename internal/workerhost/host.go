// Package workerhost adapts the dedicated offline Colima M0 sandbox. The caller
// must exclusively own this container and its trusted Docker/Colima control plane.
// It is not a generic Docker host controller or a production node provisioner.
package workerhost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

const container = "oj-m0-sandbox"

// Host binds all operations to one immutable container ID and process lifetime.
// After Stop, even a failed stop, the object permanently rejects new operations.
type Host struct {
	run         command
	id, started string
	mu          sync.Mutex
	closed      bool
	next        uint64
	active      map[uint64]context.CancelFunc
	drained     chan struct{}
}

// OpenM0 inspects the pre-existing dedicated M0 container. It never creates,
// starts, replaces or recovers a sandbox. Ownership must be assigned by the caller;
// do not open multiple Hosts or share this container with another worker.
func OpenM0(ctx context.Context) (*Host, error) { return open(ctx, runCommand) }

// OpenM0Slot opens one fixed slot with the same identity and isolation checks as
// OpenM0. The caller must hold this slot's canonical lock throughout its lifetime.
func OpenM0Slot(ctx context.Context, slot M0Slot) (*Host, error) {
	return openSlot(ctx, runCommand, slot)
}

func open(ctx context.Context, run command) (*Host, error) {
	return openSlot(ctx, run, M0Slot{})
}

func openSlot(ctx context.Context, run command, slot M0Slot) (*Host, error) {
	h := &Host{run: run, active: make(map[uint64]context.CancelFunc), drained: make(chan struct{})}
	s, err := h.inspect(ctx, slot.ContainerName())
	if err != nil {
		return nil, err
	}
	if !containerID.MatchString(s.ID) || !s.Running || s.PID <= 1 || s.Started == "" {
		return nil, errors.New("sandbox identity unavailable")
	}
	h.id, h.started = s.ID, s.Started
	if err := h.Verify(ctx, experiment.PinnedEnvironment()); err != nil {
		return nil, err
	}
	return h, nil
}

var containerID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (h *Host) docker(ctx context.Context, input io.Reader, args ...string) ([]byte, error) {
	return h.run(ctx, input, "docker", append([]string{"--context", "colima-oj-m0"}, args...)...)
}

func (h *Host) begin(ctx context.Context) (context.Context, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, nil, errors.New("sandbox host fenced")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	h.next++
	id := h.next
	h.active[id] = cancel
	return ctx, func() {
		cancel()
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.active, id)
		if h.closed && len(h.active) == 0 {
			close(h.drained)
		}
	}, nil
}

// Verify checks the fixed profile, trusted host identity and isolation settings.
// Pair it with execution.GoJudge.Verify for REST version/cgroup/seccomp evidence.
func (h *Host) Verify(ctx context.Context, p experiment.ExecutionProfile) error {
	ctx, done, err := h.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	if !reflect.DeepEqual(p, experiment.PinnedEnvironment()) {
		return errors.New("unsupported host profile")
	}
	s, err := h.inspect(ctx, h.id)
	if err != nil {
		return err
	}
	if err := h.validate(s, p); err != nil {
		return err
	}
	for _, check := range []struct {
		args []string
		want string
	}{
		{[]string{"exec", h.id, "uname", "-r"}, p.KernelRelease},
		{[]string{"image", "inspect", "--format", "{{.Os}}/{{.Architecture}}", p.ImageID}, p.Platform},
		{[]string{"exec", h.id, p.CompileArgs[0], "-dumpfullversion"}, p.CompilerVersion},
	} {
		out, err := h.docker(ctx, nil, check.args...)
		if err != nil || strings.TrimSpace(string(out)) != check.want {
			return errors.New("host execution environment mismatch")
		}
	}
	return ctx.Err()
}

// Only selected fields are read: no environment values, labels or credentials.
const inspection = `{"ID":{{json .Id}},"Image":{{json .Image}},"Network":{{json .HostConfig.NetworkMode}},"Readonly":{{json .HostConfig.ReadonlyRootfs}},"Mounts":{{len .Mounts}},"Privileged":{{json .HostConfig.Privileged}},"Memory":{{.HostConfig.Memory}},"CPUs":{{.HostConfig.NanoCpus}},"Pids":{{json .HostConfig.PidsLimit}},"Entrypoint":{{json .Config.Entrypoint}},"Cmd":{{json .Config.Cmd}},"Tmpfs":{{json .HostConfig.Tmpfs}},"Shm":{{.HostConfig.ShmSize}},"Restart":{{json .HostConfig.RestartPolicy.Name}},"PIDMode":{{json .HostConfig.PidMode}},"IPCMode":{{json .HostConfig.IpcMode}},"Running":{{.State.Running}},"PID":{{.State.Pid}},"Restarting":{{.State.Restarting}},"Paused":{{.State.Paused}},"Started":{{json .State.StartedAt}}}`

type state struct {
	ID, Image, Network, Restart, PIDMode, IPCMode, Started string
	Readonly, Privileged, Running, Restarting, Paused      bool
	Mounts, Pids, PID                                      int
	Memory, CPUs, Shm                                      int64
	Entrypoint, Cmd                                        []string
	Tmpfs                                                  map[string]string
}

func (h *Host) inspect(ctx context.Context, id string) (state, error) {
	var s state
	out, err := h.docker(ctx, nil, "inspect", "--format", inspection, id)
	if err != nil {
		return s, err
	}
	if json.Unmarshal(out, &s) != nil {
		return s, errors.New("invalid host inspection")
	}
	return s, nil
}

func (h *Host) validate(s state, p experiment.ExecutionProfile) error {
	if s.ID != h.id || s.Started != h.started || !s.Running || s.PID <= 1 || s.Restarting || s.Paused ||
		s.Image != p.ImageID || s.Network != "none" || !s.Readonly || s.Mounts != 0 || !s.Privileged ||
		s.Memory != 1<<30 || s.CPUs != 1e9 || s.Pids != 128 || s.Shm != 128<<20 ||
		s.Restart != "no" || s.PIDMode != "" || s.IPCMode != "private" || len(s.Cmd) != 0 ||
		!reflect.DeepEqual(s.Entrypoint, p.SandboxArgs) || !reflect.DeepEqual(s.Tmpfs, map[string]string{"/tmp": "rw,nosuid,size=128m"}) {
		return errors.New("sandbox identity, lifetime or isolation mismatch")
	}
	return nil
}

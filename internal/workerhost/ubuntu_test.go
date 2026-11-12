package workerhost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestUbuntuHostPinsEngineThroughStopAndRecovery(t *testing.T) {
	f, err := experiment.ParseWorkerFixture(experiment.UbuntuVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, recovery := range []bool{false, true} {
		s := goodState()
		s.Cgroupns, s.Swap = "private", 1<<30
		kernel, pidsMax := "6.8.0-106-generic", "128"
		var killed bool
		var probes []string
		run := func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
			if name != "/usr/bin/docker" || len(args) < 2 || args[0] != "--host=unix:///var/run/docker.sock" {
				t.Fatal("Ubuntu command escaped the fixed local Engine")
			}
			switch args[1] {
			case "inspect":
				if !strings.Contains(args[3], ".HostConfig.CgroupnsMode") || !strings.Contains(args[3], ".HostConfig.MemorySwap") {
					t.Fatal("Ubuntu inspection omitted runtime boundaries")
				}
				return json.Marshal(s)
			case "image":
				return []byte("linux/arm64"), nil
			case "exec":
				switch args[3] {
				case "uname":
					return []byte(kernel), nil
				case "/usr/local/bin/g++":
					return []byte("15.3.0"), nil
				case "cat":
					return []byte(map[string]string{
						"/sys/fs/cgroup/cpu.max": "100000 100000", "/sys/fs/cgroup/memory.max": "1073741824",
						"/sys/fs/cgroup/memory.swap.max": "0", "/sys/fs/cgroup/pids.max": pidsMax,
					}[args[4]]), nil
				}
			case "top":
				return []byte("PID\n42\n43\n"), nil
			case "kill":
				killed, s.Running, s.PID = true, false, 0
				return nil, nil
			case "run":
				joined := strings.Join(args, " ")
				if !killed || !strings.Contains(joined, "--pid host --network none --read-only --cap-drop ALL") ||
					!strings.Contains(joined, f.Profile().ImageID) {
					t.Fatal("PID observation did not use the pinned stopped sandbox environment")
				}
				probes = append(probes, args[len(args)-1])
				return nil, nil
			}
			return nil, errors.New("unexpected test command")
		}
		if recovery {
			if err := recoverConfiguredHost(t.Context(), &Host{run: run, fixture: f}, container, s.ID); err != nil {
				t.Fatal(err)
			}
		} else {
			h, err := openConfiguredHost(t.Context(), &Host{run: run, fixture: f}, container)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.Verify(t.Context(), experiment.PinnedEnvironment()); err == nil {
				t.Fatal("Desktop profile accepted in Ubuntu")
			}
			kernel = experiment.PinnedEnvironment().KernelRelease
			if err := h.Verify(t.Context(), f.Profile()); err == nil {
				t.Fatal("wrong kernel accepted by Ubuntu host")
			}
			kernel, pidsMax = f.Profile().KernelRelease, "max"
			if err := h.Verify(t.Context(), f.Profile()); err == nil {
				t.Fatal("ineffective process bound accepted by Ubuntu host")
			}
			pidsMax, s.Image = "128", "different-image"
			if err := h.Verify(t.Context(), f.Profile()); err == nil {
				t.Fatal("wrong image accepted by Ubuntu host")
			}
			s.Image = f.Profile().ImageID
			if err := h.Stop(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
		if !killed || strings.Join(probes, ",") != "42,43" {
			t.Fatal("stop evidence omitted observed guest PIDs")
		}
	}
}

func TestUbuntuIsolationRejectsRuntimeDrift(t *testing.T) {
	f, err := experiment.ParseWorkerFixture(experiment.UbuntuVersion)
	if err != nil {
		t.Fatal(err)
	}
	h := &Host{fixture: f}
	for _, mutate := range []func(*state){
		func(s *state) { s.Cgroupns = "host" },
		func(s *state) { s.Swap++ },
		func(s *state) { s.PortBindings = 1 },
		func(s *state) { s.PublishAllPorts = true },
	} {
		s := goodState()
		s.Cgroupns, s.Swap = "private", 1<<30
		mutate(&s)
		if err := h.validateIsolation(s, f.Profile()); err == nil {
			t.Fatal("changed Ubuntu isolation accepted")
		}
	}
}

func TestUbuntuRejectsGuestHostShares(t *testing.T) {
	const local = "25 1 8:1 / / rw - ext4 /dev/vda1 rw\n"
	if !isolatedUbuntuMounts(local) || isolatedUbuntuMounts("") || isolatedUbuntuMounts(local+"malformed\n") {
		t.Fatal("guest mount evidence was not required")
	}
	for _, fs := range []string{"virtiofs", "9p", "fuse.sshfs", "sshfs"} {
		if isolatedUbuntuMounts(local + "26 25 0:33 / /mnt/host rw - " + fs + " mount rw\n") {
			t.Fatalf("host share type %q allowed", fs)
		}
	}
}

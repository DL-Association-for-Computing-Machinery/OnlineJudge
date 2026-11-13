package workerhost

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

// OpenUbuntuM0 opens only the fixed primary slot in the dedicated local guest.
// The caller must hold the same canonical ownership lock used by Desktop M0.
func OpenUbuntuM0(ctx context.Context) (*Host, error) {
	h, err := ubuntuHost(ctx)
	if err != nil {
		return nil, err
	}
	return openConfiguredHost(ctx, h, container)
}

func ubuntuHost(ctx context.Context) (*Host, error) {
	fixture, err := experiment.ParseWorkerFixture(experiment.UbuntuVersion)
	if err != nil {
		return nil, err
	}
	h := &Host{run: ubuntuCommand, fixture: fixture}
	if err := h.verifyUbuntuGuest(ctx, fixture.Profile()); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *Host) nativeUbuntu() bool {
	return h.fixture.Version() == experiment.UbuntuVersion || h.tier == experiment.ReferenceAPlusBUbuntu
}

// CheckUbuntuAPlusBHost verifies the dedicated guest before the CLI dirties its lock.
func CheckUbuntuAPlusBHost(ctx context.Context) error {
	h := &Host{run: ubuntuCommand, tier: experiment.ReferenceAPlusBUbuntu}
	p, _, err := h.configuration()
	if err != nil {
		return err
	}
	return h.verifyUbuntuGuest(ctx, p)
}

func (h *Host) verifyUbuntuGuest(ctx context.Context, p experiment.ExecutionProfile) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" || os.Geteuid() != 0 {
		return errors.New("the Ubuntu experiments require the dedicated Linux arm64 VM owner")
	}
	virt, err := ubuntuCommand(ctx, nil, "/usr/bin/systemd-detect-virt", "--vm")
	if err != nil || strings.TrimSpace(string(virt)) != "apple" {
		return errors.New("the Ubuntu experiments require the pinned local VM hypervisor")
	}
	mounts, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil || !isolatedUbuntuMounts(string(mounts)) {
		return errors.New("the Ubuntu experiments require a guest without host shares")
	}
	kernel, err := ubuntuCommand(ctx, nil, "/usr/bin/uname", "-r")
	if err != nil || strings.TrimSpace(string(kernel)) != p.KernelRelease {
		return errors.New("the Ubuntu experiments require the pinned VM kernel")
	}
	engine, err := h.docker(ctx, nil, "info", "--format", "{{.OSType}}/{{.Architecture}}/{{.CgroupVersion}}")
	if err != nil || strings.TrimSpace(string(engine)) != "linux/aarch64/2" {
		return errors.New("the Ubuntu experiments require the fixed local cgroup v2 Engine")
	}
	return ctx.Err()
}

func isolatedUbuntuMounts(mounts string) bool {
	if strings.TrimSpace(mounts) == "" {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(mounts), "\n") {
		_, tail, ok := strings.Cut(line, " - ")
		fstype, _, space := strings.Cut(tail, " ")
		if !ok || !space || fstype == "" {
			return false
		}
		switch fstype {
		case "virtiofs", "9p", "fuse.sshfs", "sshfs":
			return false
		}
	}
	return true
}

func ubuntuCommand(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
	// No inherited Docker context, client configuration, proxy or executable path.
	return runCommandWithEnv(ctx, input, ubuntuCommandEnv(), name, args...)
}

func ubuntuCommandTo(ctx context.Context, input io.Reader, output io.Writer, maximum int64, name string, args ...string) error {
	return runCommandToWithEnv(ctx, input, output, maximum, ubuntuCommandEnv(), name, args...)
}

func ubuntuCommandEnv() []string {
	return []string{"HOME=/", "PATH=/usr/bin:/bin", "LANG=C"}
}

func (h *Host) verifyUbuntuLimits(ctx context.Context) error {
	for _, item := range [...]struct{ name, value string }{
		{"cpu.max", "100000 100000"},
		{"memory.max", "1073741824"},
		{"memory.swap.max", "0"},
		{"pids.max", "128"},
	} {
		out, err := h.docker(ctx, nil, "exec", h.id, "cat", "/sys/fs/cgroup/"+item.name)
		if err != nil || strings.TrimSpace(string(out)) != item.value {
			return errors.New("effective sandbox cgroup limits mismatch for Ubuntu")
		}
	}
	return nil
}

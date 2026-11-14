package workerhost

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

const (
	trialAMD64Slot   = "/oj-aplusb-trial-amd64-sandbox"
	trialAMD64Owner  = "onlinejudge.trial-aplusb-amd64-test"
	trialAMD64Config = "sha256:fe1a97991b090d9232fb8b086139925debe96dce834fe16def8c7f3373a2f4f9"
)

// CheckAPlusBTrialAMD64Slot inspects a previously provisioned candidate by its
// independently recorded full ID. It cannot start it or admit it for execution.
func CheckAPlusBTrialAMD64Slot(ctx context.Context, id string) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" || os.Geteuid() != 0 {
		return errors.New("candidate preflight requires the dedicated Linux amd64 operator")
	}
	return checkAPlusBTrialAMD64Slot(ctx, ubuntuCommand, id)
}

type trialAMD64Inspection struct {
	state
	Name, Owner, Status string
	AutoRemove          bool
}

func checkAPlusBTrialAMD64Slot(ctx context.Context, run command, id string) error {
	if !containerID.MatchString(id) {
		return errors.New("candidate preflight requires the original full container ID")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	p := experiment.APlusBTrialAMD64Candidate().Runtime
	kernel, err := run(ctx, nil, "/usr/bin/uname", "-r")
	if err != nil || strings.TrimSpace(string(kernel)) != p.KernelRelease {
		return errors.New("candidate host kernel mismatch")
	}
	inspect := func(args ...string) ([]byte, error) {
		return run(ctx, nil, "/usr/bin/docker", append([]string{"--host=unix:///var/run/docker.sock"}, args...)...)
	}
	engine, err := inspect("info", "--format", "{{.OSType}}/{{.Architecture}}/{{.CgroupVersion}}")
	if err != nil || strings.TrimSpace(string(engine)) != "linux/x86_64/2" {
		return errors.New("candidate requires the native Linux x86_64 cgroup v2 Engine")
	}
	format := strings.TrimSuffix(inspection, "}") + `,"Cgroupns":{{json .HostConfig.CgroupnsMode}},"Swap":{{.HostConfig.MemorySwap}},"PortBindings":{{len .HostConfig.PortBindings}},"PublishAllPorts":{{.HostConfig.PublishAllPorts}},"AutoRemove":{{.HostConfig.AutoRemove}},"Name":{{json .Name}},"Owner":{{json (index .Config.Labels "owner")}},"Status":{{json .State.Status}}}`
	raw, err := inspect("container", "inspect", "--format", format, id)
	var s trialAMD64Inspection
	if err != nil || json.Unmarshal(raw, &s) != nil || s.ID != id ||
		s.Name != trialAMD64Slot || s.Owner != trialAMD64Owner ||
		(s.Status != "created" && s.Status != "exited") || s.Running || s.PID != 0 || s.Paused || s.Restarting ||
		s.AutoRemove || s.Cgroupns != "private" || s.Swap != 1<<30 || s.PortBindings != 0 || s.PublishAllPorts ||
		(s.Image != trialAMD64Config && s.Image != p.ImageID) {
		return errors.New("candidate slot identity, ownership or stopped state mismatch")
	}
	// Docker image stores may report the manifest or its fixed config identity.
	p.ImageID = s.Image
	if err := validateIsolation(s.state, p); err != nil {
		return err
	}
	image, err := inspect("image", "inspect", "--format", "{{.Id}} {{.Os}}/{{.Architecture}}", s.Image)
	if err != nil || strings.TrimSpace(string(image)) != s.Image+" "+p.Platform {
		return errors.New("candidate image identity or platform mismatch")
	}
	return ctx.Err()
}

package workerhost

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestTrialAMD64PreflightOnlyInspectsPinnedStoppedSlot(t *testing.T) {
	p := experiment.APlusBTrialAMD64Candidate().Runtime
	base := trialAMD64Inspection{state: goodState(), Name: trialAMD64Slot, Owner: trialAMD64Owner, Status: "exited"}
	base.Image, base.Entrypoint = p.ImageID, p.SandboxArgs
	base.Running, base.PID, base.Cgroupns, base.Swap = false, 0, "private", 1<<30
	for _, tc := range []struct {
		name string
		edit func(*trialAMD64Inspection, *string, *string, *string)
	}{
		{"manifest identity", nil},
		{"config identity", func(s *trialAMD64Inspection, _, _, _ *string) { s.Image = trialAMD64Config }},
		{"running", func(s *trialAMD64Inspection, _, _, _ *string) { s.Running = true }},
		{"dead", func(s *trialAMD64Inspection, _, _, _ *string) { s.Status = "dead" }},
		{"wrong container", func(s *trialAMD64Inspection, _, _, _ *string) { s.ID = strings.Repeat("b", 64) }},
		{"wrong owner", func(s *trialAMD64Inspection, _, _, _ *string) { s.Owner = "other" }},
		{"wrong slot", func(s *trialAMD64Inspection, _, _, _ *string) { s.Name = "/oj-aplusb-ubuntu-sandbox" }},
		{"host network", func(s *trialAMD64Inspection, _, _, _ *string) { s.Network = "host" }},
		{"mounted volume", func(s *trialAMD64Inspection, _, _, _ *string) { s.Mounts = 1 }},
		{"published port", func(s *trialAMD64Inspection, _, _, _ *string) { s.PortBindings = 1 }},
		{"cgroup drift", func(s *trialAMD64Inspection, _, _, _ *string) { s.Cgroupns = "host" }},
		{"memory drift", func(s *trialAMD64Inspection, _, _, _ *string) { s.Memory++ }},
		{"automatic removal", func(s *trialAMD64Inspection, _, _, _ *string) { s.AutoRemove = true }},
		{"different image", func(s *trialAMD64Inspection, _, _, _ *string) { s.Image = experiment.PinnedEnvironment().ImageID }},
		{"different kernel", func(_ *trialAMD64Inspection, k, _, _ *string) { *k = "6.8.0-other" }},
		{"different engine", func(_ *trialAMD64Inspection, _, e, _ *string) { *e = "linux/x86_64/1" }},
		{"wrong architecture", func(_ *trialAMD64Inspection, _, _, a *string) { *a = "linux/arm64" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			kernel, engine, arch := p.KernelRelease, "linux/x86_64/2", p.Platform
			if tc.edit != nil {
				tc.edit(&s, &kernel, &engine, &arch)
			}
			var calls []string
			run := func(_ context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
				if input != nil {
					t.Fatal("preflight supplied command input")
				}
				if name == "/usr/bin/uname" && slices.Equal(args, []string{"-r"}) {
					calls = append(calls, "uname")
					return []byte(kernel), nil
				}
				if name != "/usr/bin/docker" || len(args) < 3 || args[0] != "--host=unix:///var/run/docker.sock" {
					t.Fatalf("not the fixed local Engine: %s %v", name, args)
				}
				calls = append(calls, args[1])
				switch {
				case args[1] == "info":
					return []byte(engine), nil
				case args[1] == "container" && args[2] == "inspect" && args[len(args)-1] == base.ID:
					return json.Marshal(s)
				case args[1] == "image" && args[2] == "inspect" && args[len(args)-1] == s.Image:
					return []byte(s.Image + " " + arch), nil
				}
				t.Fatalf("unexpected command: %v", args)
				return nil, nil
			}
			want := tc.name == "manifest identity" || tc.name == "config identity"
			if err := checkAPlusBTrialAMD64Slot(t.Context(), run, base.ID); (err == nil) != want {
				t.Fatalf("unexpected preflight result: %v", err)
			}
			if want && !slices.Equal(calls, []string{"uname", "info", "container", "image"}) {
				t.Fatalf("unexpected read-only sequence: %v", calls)
			}
			if err := checkAPlusBTrialAMD64Slot(t.Context(), nil, strings.TrimPrefix(trialAMD64Slot, "/")); err == nil {
				t.Fatal("mutable slot alias accepted")
			}
		})
	}
}

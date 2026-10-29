//go:build sandbox

package sandbox_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

// Inspect before submitting any program, even when a mutable local tag was moved.
func verifyEnvironment(t *testing.T) {
	t.Helper()
	if os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" {
		t.Fatal("explicit OJ_SANDBOX_EXPERIMENT=1 required")
	}
	p := experiment.PinnedEnvironment()
	docker := func(args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "docker", append([]string{"--context", "desktop-linux"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("environment inspection failed: %v: %s", err, out)
		}
		return out
	}
	// Read only the identity and isolation fields, never the container environment.
	const fields = `{{json .Image}} {{json .HostConfig.NetworkMode}} {{json .HostConfig.ReadonlyRootfs}} {{len .Mounts}} {{json .HostConfig.Privileged}} {{.HostConfig.Memory}} {{.HostConfig.NanoCpus}} {{.HostConfig.PidsLimit}}`
	got := strings.TrimSpace(string(docker("inspect", "--format", fields, container)))
	want := `"` + p.ImageID + `" "none" true 0 true 1073741824 1000000000 128`
	if got != want {
		t.Fatalf("container identity/isolation mismatch: %s", got)
	}
	entrypoint, err := json.Marshal(p.SandboxArgs)
	if err != nil {
		t.Fatal(err)
	}
	got = strings.TrimSpace(string(docker("inspect", "--format", "{{json .Config.Entrypoint}} {{json .Config.Cmd}} {{json .HostConfig.Tmpfs}} {{.HostConfig.ShmSize}}", container)))
	if got != string(entrypoint)+` null {"/tmp":"rw,nosuid,size=128m"} 134217728` {
		t.Fatal("sandbox invocation or temporary storage budget mismatch")
	}
	if got := strings.TrimSpace(string(docker("exec", container, "uname", "-r"))); got != p.KernelRelease {
		t.Fatalf("kernel mismatch: %s", got)
	}
	if got := strings.TrimSpace(string(docker("image", "inspect", "--format", "{{.Os}}/{{.Architecture}}", p.ImageID))); got != p.Platform {
		t.Fatalf("platform mismatch: %s", got)
	}
	if got := strings.TrimSpace(string(docker("exec", container, p.CompileArgs[0], "-dumpfullversion"))); got != p.CompilerVersion {
		t.Fatalf("compiler mismatch: %s", got)
	}
	var version struct {
		Build    string `json:"buildVersion"`
		OS       string `json:"os"`
		Platform string `json:"platform"`
	}
	if err := json.Unmarshal(api(t, "GET", "/version", nil), &version); err != nil {
		t.Fatal(err)
	}
	if version.Build != p.SandboxVersion || version.OS+"/"+version.Platform != p.Platform {
		t.Fatalf("sandbox version mismatch: %+v", version)
	}
	t.Logf("verified profile %s image %s compiler %s sandbox %s", p.ID, p.ImageID, p.CompilerVersion, p.SandboxVersion)
}

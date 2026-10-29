package workerhost

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"

// PIDProbeArgs observes the Docker VM process namespace without privilege,
// network, mounts or a Docker socket. Callers pass only validated numeric PIDs.
// Checking /proc/1 first prevents a missing procfs from masquerading as exit.
func PIDProbeArgs(pid string) []string {
	return []string{"--context", "desktop-linux", "run", "--rm", "--pull", "never",
		"--pid", "host", "--network", "none", "--read-only", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", "--entrypoint", "/bin/sh",
		experiment.PinnedEnvironment().ImageID, "-ec", "test -d /proc/1 && test ! -e \"/proc/$1\"", "probe", pid}
}

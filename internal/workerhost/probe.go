package workerhost

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"

const (
	probeProcUnavailable = 41
	probePIDPresent      = 42
)

// PIDProbeArgs observes the Docker VM process namespace without privilege,
// network, mounts or a Docker socket. Callers pass only validated numeric PIDs.
// Checking /proc/1 first prevents a missing procfs from masquerading as exit.
func PIDProbeArgs(pid string) []string {
	return append([]string{"--context", "desktop-linux"}, pidProbeArgs(experiment.PinnedEnvironment().ImageID, pid)...)
}

func pidProbeArgs(image, pid string) []string {
	return []string{"run", "--rm", "--pull", "never",
		"--pid", "host", "--network", "none", "--read-only", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", "--entrypoint", "/bin/sh",
		image, "-ec", "test -d /proc/1 || exit 41; if test ! -e \"/proc/$1\"; then exit 0; else case $? in 1) exit 42;; *) exit 43;; esac; fi", "probe", pid}
}

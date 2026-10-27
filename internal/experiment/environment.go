package experiment

// ExecutionProfile describes the exact local M0 artifact and command budgets.
// ImageID is a Docker config digest, not a distributable registry manifest digest.
// A rebuild or another architecture requires a new profile and material version.
type ExecutionProfile struct {
	ID               string   `json:"id"`
	Platform         string   `json:"platform"`
	KernelRelease    string   `json:"kernel_release"`
	ImageID          string   `json:"image_id"`
	DockerfileSHA256 string   `json:"dockerfile_sha256"`
	CompilerVersion  string   `json:"compiler_version"`
	SandboxVersion   string   `json:"sandbox_version"`
	SandboxArgs      []string `json:"sandbox_args"`
	CompileArgs      []string `json:"compile_args"`
	RunArgs          []string `json:"run_args"`
	Env              []string `json:"env"`
	Compile          Limits   `json:"compile"`
	Run              Limits   `json:"run"`
	OutputBytes      int      `json:"output_bytes"`
	ExecutableBytes  int      `json:"executable_bytes"`
}

// Limits uses nanoseconds for CPU/wall time and bytes for memory.
type Limits struct {
	CPU       int64 `json:"cpu_ns"`
	Wall      int64 `json:"wall_ns"`
	Memory    int64 `json:"memory_bytes"`
	Processes int   `json:"processes"`
}

// PinnedEnvironment returns fresh values so callers cannot mutate later manifests.
// These settings are for the dedicated, offline Linux arm64 experiment only.
func PinnedEnvironment() ExecutionProfile {
	return ExecutionProfile{
		ID: "m0-gnu17-linux-arm64-v1", Platform: "linux/arm64",
		KernelRelease:    "6.8.0-100-generic",
		ImageID:          "sha256:fa4d3b785c8245ae856b3bda4687deeb74fbd9615a0de6df3c93ab6151717661",
		DockerfileSHA256: "195f6b59a9b42c5a8626291aba22c626b7df4e137aa8517093b1411e9c141ddc",
		CompilerVersion:  "15.3.0", SandboxVersion: "v1.13.3",
		SandboxArgs: []string{"/opt/go-judge", "-http-addr", "127.0.0.1:5050", "-parallelism", "1", "-no-fallback", "-file-timeout", "1m", "-tmp-fs-param", "size=64m,nr_inodes=4k", "-output-limit", "1048576", "-copy-out-limit", "1048576"},
		CompileArgs: []string{"/usr/local/bin/g++", "-std=gnu++17", "-O2", "-pipe", "main.cpp", "-o", "main"},
		RunArgs:     []string{"main"},
		Env:         []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C", "LD_LIBRARY_PATH=/usr/local/lib64"},
		Compile:     Limits{CPU: 10000000000, Wall: 15000000000, Memory: 512 << 20, Processes: 16},
		Run:         Limits{CPU: 200000000, Wall: 2000000000, Memory: 32 << 20, Processes: 16},
		OutputBytes: 4096, ExecutableBytes: 1 << 20,
	}
}

package experiment

const (
	pythonInterpreter = "/usr/local/bin/python3.14"
	// PythonImageID pins the combined local GCC, Temurin and CPython executor.
	PythonImageID = "sha256:e6b23d40935394adff7db3d21ac237a5921cc3b7e403acccc88158f759dfc2a3"
)

// PythonTextBase keeps the existing language profiles on the combined image.
func PythonTextBase(p ExecutionProfile) ExecutionProfile {
	p.ID += "-cpython314-image-v1"
	p.ImageID = PythonImageID
	p.DockerfileSHA256 = "eec41e3657a82ea5f622fb249f4c1fe8c1a29e8a5a5e4aa9a52891f6371d5811"
	return p
}

// Python3Profile compiles a single source file without executing it. Each point
// runs the bounded bytecode artifact in a fresh isolated interpreter.
func Python3Profile(p ExecutionProfile) ExecutionProfile {
	p.ID += "-python314-v1"
	p.CompilerVersion = "Python 3.14.10"
	p.CompileArgs = []string{pythonInterpreter, "-I", "-S", "-B", "-c",
		"import py_compile; py_compile.compile('main.py', cfile='main.pyc', dfile='main.py', doraise=True, invalidation_mode=py_compile.PycInvalidationMode.CHECKED_HASH)"}
	p.RunArgs = []string{pythonInterpreter, "-I", "-S", "-B", "main.pyc"}
	return p
}

// IsPython3Profile identifies the already validated Python compilation contract.
func IsPython3Profile(p ExecutionProfile) bool {
	return p.ImageID == PythonImageID && len(p.CompileArgs) > 0 && p.CompileArgs[0] == pythonInterpreter
}

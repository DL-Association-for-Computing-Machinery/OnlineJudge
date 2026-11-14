package experiment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

const (
	// JavaImageID is the pinned local Ubuntu arm64 executor with GCC and Temurin.
	JavaImageID  = "sha256:fd431defcef3236ceb9e17b40d906ef0f8e7f3061398b668ad4ca5e527932341"
	javaCompiler = "/usr/local/bin/oj-compile-java21"
)

// JavaTextBase retains the native profiles on the same pinned JDK image.
func JavaTextBase(p ExecutionProfile) ExecutionProfile {
	p.ID += "-temurin21-v1"
	p.ImageID = JavaImageID
	p.DockerfileSHA256 = "3a242a9c4b7996da2faa402acf3b39f688661d8ed8c357286138a7a3a4ffa00c"
	return p
}

// Java21Profile pins compilation, the fresh JVM invocation and its resource envelope.
func Java21Profile(p ExecutionProfile) ExecutionProfile {
	p.ID += "-java21-v1"
	p.CompilerVersion = "javac 21.0.13"
	p.CompileArgs = []string{javaCompiler}
	p.RunArgs = []string{"/usr/local/oj-jdk/bin/java", "-Xint", "-XX:ActiveProcessorCount=1", "-XX:+UseSerialGC", "-XX:-UsePerfData",
		"-Xmx64m", "-Xss256k", "-cp", "main.jar", "Main"}
	p.Run = Limits{CPU: 1500000000, Wall: 5000000000, Memory: 192 << 20, Processes: 32}
	p.Compile.Processes = 32
	return p
}

// IsJava21Profile identifies the already validated Java compilation contract.
func IsJava21Profile(p ExecutionProfile) bool {
	return p.ImageID == JavaImageID && len(p.CompileArgs) == 1 && p.CompileArgs[0] == javaCompiler
}

// JavaTextExecutionSHA256 binds the actual contestant profile separately
// from the published materials' reference compiler and transport digest.
func JavaTextExecutionSHA256(p ExecutionProfile) string {
	raw, _ := json.Marshal(p)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

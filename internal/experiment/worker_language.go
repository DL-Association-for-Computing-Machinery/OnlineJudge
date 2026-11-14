package experiment

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"

// SupportsLanguage limits C17 to the published text material runtimes.
func (f WorkerFixture) SupportsLanguage(language string) bool {
	return language == submission.LanguageGNUCPP17 || f.UsesCachedText() && language == submission.LanguageC17
}

// ProfileForLanguage derives the compiler from the trusted fixture, never the submission.
// Callers must check SupportsLanguage before resolving untrusted language keys.
func (f WorkerFixture) ProfileForLanguage(language string) ExecutionProfile {
	profile := f.Profile()
	if language == submission.LanguageC17 && f.UsesCachedText() {
		return C17Profile(profile)
	}
	return profile
}

// C17Profile selects GCC and the fixed C17 toolchain without changing isolation or budgets.
func C17Profile(profile ExecutionProfile) ExecutionProfile {
	profile.ID += "-c17-v1"
	profile.CompileArgs = []string{"/usr/local/bin/gcc", "-std=c17", "-pedantic-errors", "-O2", "-pipe", "main.c", "-o", "main"}
	return profile
}

// SingleSourceName returns the internal filename for the trusted compiler profile.
func SingleSourceName(profile ExecutionProfile) string {
	if len(profile.CompileArgs) > 0 && profile.CompileArgs[0] == "/usr/local/bin/gcc" {
		return "main.c"
	}
	return "main.cpp"
}

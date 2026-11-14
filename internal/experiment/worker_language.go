package experiment

import (
	"reflect"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// SupportsLanguage limits extra languages to published text material runtimes.
func (f WorkerFixture) SupportsLanguage(language string) bool {
	return language == submission.LanguageGNUCPP17 || f.UsesCachedText() && submission.SupportedLanguage(language)
}

// ProfileForLanguage derives the compiler from the trusted fixture, never the submission.
// Callers must check SupportsLanguage before resolving untrusted language keys.
func (f WorkerFixture) ProfileForLanguage(language string) ExecutionProfile {
	profile := f.Profile()
	if f.UsesCachedText() {
		switch language {
		case submission.LanguageC17:
			return C17Profile(profile)
		case submission.LanguageGNUCPP20:
			return GNUCPP20Profile(profile)
		}
	}
	return profile
}

// C17Profile selects GCC and the fixed C17 toolchain without changing isolation or budgets.
func C17Profile(profile ExecutionProfile) ExecutionProfile {
	profile.ID += "-c17-v1"
	profile.CompileArgs = []string{"/usr/local/bin/gcc", "-std=c17", "-pedantic-errors", "-O2", "-pipe", "main.c", "-o", "main"}
	return profile
}

// GNUCPP20Profile pins the C++20 compiler mode, source name and existing budgets.
func GNUCPP20Profile(profile ExecutionProfile) ExecutionProfile {
	profile.ID += "-gnu20-v1"
	profile.CompileArgs = []string{"/usr/local/bin/g++", "-std=gnu++20", "-O2", "-pipe", "main.cpp", "-o", "main"}
	return profile
}

// MatchesPublishedTextProfile admits only full equality with a fixed variant.
func MatchesPublishedTextProfile(base, candidate ExecutionProfile) bool {
	return reflect.DeepEqual(candidate, C17Profile(base)) || reflect.DeepEqual(candidate, GNUCPP20Profile(base))
}

// SingleSourceName returns the internal filename for the trusted compiler profile.
func SingleSourceName(profile ExecutionProfile) string {
	if len(profile.CompileArgs) > 0 && profile.CompileArgs[0] == "/usr/local/bin/gcc" {
		return "main.c"
	}
	return "main.cpp"
}

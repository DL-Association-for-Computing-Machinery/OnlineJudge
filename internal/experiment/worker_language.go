package experiment

import (
	"errors"
	"reflect"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// SupportsLanguage limits extra languages to published text material runtimes.
func (f WorkerFixture) SupportsLanguage(language string) bool {
	if language == submission.LanguageJava21 {
		return f.Java21Enabled()
	}
	if language == submission.LanguagePython3 {
		return f.Python3Enabled()
	}
	return language == submission.LanguageGNUCPP17 || f.UsesCachedText() && submission.SupportedLanguage(language)
}

// Java21Enabled reports whether this queue uses the admitted JDK image.
func (f WorkerFixture) Java21Enabled() bool { return f.java21 }

// Python3Enabled reports whether the pinned Ubuntu text interpreter is in use.
func (f WorkerFixture) Python3Enabled() bool { return f.python3 }

// WithPython3 opts in without changing the published material binding.
func (f WorkerFixture) WithPython3() (WorkerFixture, error) {
	if !f.UsesCachedText() || f.APlusBTier() != ReferenceAPlusBUbuntu {
		return WorkerFixture{}, errors.New("the Python runtime requires a pinned Ubuntu text fixture")
	}
	f.python3 = true
	return f, nil
}

// WithJava21 enables the fixed Java runtime for the Ubuntu text fixture only.
func (f WorkerFixture) WithJava21() (WorkerFixture, error) {
	if !f.UsesCachedText() || f.APlusBTier() != ReferenceAPlusBUbuntu {
		return WorkerFixture{}, errors.New("the Java runtime requires a pinned Ubuntu text fixture")
	}
	f.java21 = true
	return f, nil
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
		case submission.LanguageJava21:
			if f.java21 {
				return Java21Profile(profile)
			}
		case submission.LanguagePython3:
			if f.python3 {
				return Python3Profile(profile)
			}
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
	return reflect.DeepEqual(candidate, C17Profile(base)) || reflect.DeepEqual(candidate, GNUCPP20Profile(base)) ||
		javaImage(base.ImageID) && reflect.DeepEqual(candidate, Java21Profile(base)) ||
		base.ImageID == PythonImageID && reflect.DeepEqual(candidate, Python3Profile(base))
}

// SingleSourceName returns the internal filename for the trusted compiler profile.
func SingleSourceName(profile ExecutionProfile) string {
	if IsJava21Profile(profile) {
		return "Main.java"
	}
	if IsPython3Profile(profile) {
		return "main.py"
	}
	if len(profile.CompileArgs) > 0 && profile.CompileArgs[0] == "/usr/local/bin/gcc" {
		return "main.c"
	}
	return "main.cpp"
}

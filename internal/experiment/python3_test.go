package experiment

import (
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

func TestPythonOptInKeepsJavaGatedAndRejectsOtherRuntimeProfiles(t *testing.T) {
	if _, err := (WorkerFixture{}).WithPython3(); err == nil {
		t.Fatal("legacy Worker admitted Python")
	}
	f := WorkerFixture{aplusb: "pinned manifest", aplusbTier: ReferenceAPlusBUbuntu}
	old := f.Profile()
	onlyJava, err := f.WithJava21()
	if err != nil || onlyJava.SupportsLanguage(submission.LanguagePython3) {
		t.Fatal("Java opt-in admitted Python")
	}
	python, err := f.WithPython3()
	if err != nil || !python.SupportsLanguage(submission.LanguagePython3) ||
		python.SupportsLanguage(submission.LanguageJava21) || string(python.Manifest()) != string(f.Manifest()) {
		t.Fatal("Python must be independently enabled on the same fixture")
	}
	both, err := python.WithJava21()
	if err != nil || !both.SupportsLanguage(submission.LanguageJava21) ||
		!IsJava21Profile(both.ProfileForLanguage(submission.LanguageJava21)) {
		t.Fatal("combined image did not retain its opt-in Java runtime")
	}
	base := python.Profile()
	profile := python.ProfileForLanguage(submission.LanguagePython3)
	if !MatchesPublishedTextProfile(base, profile) || MatchesPublishedTextProfile(old, Python3Profile(old)) {
		t.Fatal("Python accepted outside the admitted image or rejected inside it")
	}
	profile.Run.Memory++
	if MatchesPublishedTextProfile(base, profile) {
		t.Fatal("Python profile with altered memory limit was admitted")
	}
}

package submission_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

func TestSourceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"empty", "", false}, {"exact budget", strings.Repeat("x", submission.MaxSourceBytes), true},
		{"over budget", strings.Repeat("x", submission.MaxSourceBytes+1), false},
		{"nul", "int\x00main", false}, {"malformed utf8", string([]byte{0xff}), false},
		{"chinese comment", "// 中文注释\nint main() {}\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := submission.Validate("subject", "training", "valid-key-00000001", submission.Request{MaterialVersion: "v1", Language: "gnu++17", Source: tc.source})
			if (err == nil) != tc.valid || (err != nil && !errors.Is(err, submission.ErrInvalid)) {
				t.Fatalf("validation: %v", err)
			}
		})
	}
}

func TestDigestIncludesEveryExecutionInput(t *testing.T) {
	base := submission.Request{MaterialVersion: "v1", Language: "gnu++17", Source: "int main() {}\n"}
	changes := []submission.Request{base, base, base}
	changes[0].MaterialVersion = "v2"
	changes[1].Language = "gnu++20"
	changes[2].Source = strings.TrimSuffix(base.Source, "\n")
	for _, changed := range changes {
		if submission.Digest(base) == submission.Digest(changed) {
			t.Fatal("execution input ignored by fingerprint")
		}
	}
}

package apiconfig

import (
	"strings"
	"testing"
)

func TestTrainingHistoryIsOptIn(t *testing.T) {
	for _, key := range []string{"training_history", "training_submission_detail"} {
		t.Run(key, func(t *testing.T) { testTrainingOptIn(t, key) })
	}
}

func testTrainingOptIn(t *testing.T, key string) {
	t.Helper()
	for _, test := range []struct {
		name, value  string
		enabled, bad bool
	}{
		{name: "absent"},
		{name: "disabled", value: "false"},
		{name: "enabled", value: "true", enabled: true},
		{name: "wrong type", value: `"true"`, bad: true},
		{name: "null", value: "null", bad: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := validDocument
			if test.value != "" {
				input = strings.Replace(input, "{", `{"`+key+`":`+test.value+",", 1)
			}
			cfg, err := Parse([]byte(input))
			if (err != nil) != test.bad || cfg.Runtime.TrainingHistory != (test.enabled && key == "training_history") ||
				cfg.Runtime.TrainingSubmissionDetail != (test.enabled && key == "training_submission_detail") {
				t.Fatal("training read flags did not obey the independent opt-in contract")
			}
		})
	}
}

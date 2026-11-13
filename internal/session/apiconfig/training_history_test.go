package apiconfig

import (
	"strings"
	"testing"
)

func TestTrainingHistoryIsOptIn(t *testing.T) {
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
				input = strings.Replace(input, "{", `{"training_history":`+test.value+",", 1)
			}
			cfg, err := Parse([]byte(input))
			if (err != nil) != test.bad || cfg.Runtime.TrainingHistory != test.enabled {
				t.Fatal("training history flag did not obey the opt-in contract")
			}
		})
	}
}

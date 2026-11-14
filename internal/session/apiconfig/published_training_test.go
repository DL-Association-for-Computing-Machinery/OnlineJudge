package apiconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublishedTrainingConfiguration(t *testing.T) {
	for _, name := range []string{"two-pins", "duplicate", "missing-history", "legacy-intake", "relative-root", "unknown-queue"} {
		t.Run(name, func(t *testing.T) {
			var w map[string]any
			if err := json.Unmarshal([]byte(validDocument), &w); err != nil {
				t.Fatal(err)
			}
			id := strings.Repeat("a", 64)
			pins := []map[string]string{{"version": "aplusb-" + id, "sha256": id}, {"version": "local-text-" + id, "sha256": id}}
			p := map[string]any{"material_root": "/statements", "max_in_flight": 2, "manifest_pins": pins}
			w["training_history"], w["training_submission_detail"], w["published_training"] = true, true, p
			switch name {
			case "duplicate":
				pins[1] = pins[0]
			case "missing-history":
				delete(w, "training_history")
			case "legacy-intake":
				w["local_training_intake"] = true
			case "relative-root":
				p["material_root"] = "statements"
			case "unknown-queue":
				pins[0]["version"] = "m0-sum-v1"
			}
			body, err := json.Marshal(w)
			if err != nil {
				t.Fatal(err)
			}
			c, err := Parse(body)
			if name == "two-pins" {
				if err != nil || c.Runtime.PublishedTraining == nil || len(c.Runtime.PublishedTraining.ManifestPins) != 2 || c.Runtime.PrivateDrafts != nil {
					t.Fatalf("standalone configuration rejected: %v", err)
				}
			} else if err == nil {
				t.Fatal("invalid published training configuration accepted")
			}
		})
	}
}

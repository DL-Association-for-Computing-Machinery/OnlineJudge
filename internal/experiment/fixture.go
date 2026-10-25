// Package experiment contains controlled M0 inputs, never real training accounts.
package experiment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Version names the synthetic A+B fixture; changing material requires a new version.
const Version = "m0-sum-v1"

// Manifest pins the two ordered test inputs and answers by SHA-256.
// It is acceptance-test material only: no validated compiler/runtime image exists yet.
func Manifest() []byte {
	hash := func(value string) string { sum := sha256.Sum256([]byte(value)); return hex.EncodeToString(sum[:]) }
	data, _ := json.Marshal(map[string]any{
		"version": Version, "origin": "synthetic-project-fixture", "language": "gnu++17",
		"checker": "ascii-tokens-v1", "case_sensitive": false,
		"execution_environment": "unvalidated",
		"tests": []map[string]string{
			{"id": "01", "input_sha256": hash("1 2\n"), "answer_sha256": hash("3\n")},
			{"id": "02", "input_sha256": hash("-7 4\n"), "answer_sha256": hash("-3\n")},
		},
	})
	return data
}

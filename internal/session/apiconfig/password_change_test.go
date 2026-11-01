package apiconfig

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func passwordChangeWireFixture() map[string]any {
	return map[string]any{
		"minimum_characters": 15, "blocklist_file": "/config/weak-passwords.json",
		"global":  map[string]any{"count": 11, "window": "1m"},
		"source":  map[string]any{"count": 7, "window": "2m"},
		"subject": map[string]any{"count": 3, "window": "3m"},
	}
}

func passwordChangeConfigBody(t *testing.T, change any, capacity uint32) []byte {
	t.Helper()
	var doc map[string]any
	if json.Unmarshal([]byte(validDocument), &doc) != nil {
		t.Fatal("base configuration fixture invalid")
	}
	doc["password_change"] = change
	if capacity != 0 {
		doc["password"].(map[string]any)["limits"].(map[string]any)["max_password_bytes"] = capacity
	}
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatal("optional configuration fixture could not encode")
	}
	return body
}

func TestPasswordChangeConfigurationIsExplicitAndOptional(t *testing.T) {
	absent, err := Parse([]byte(validDocument))
	if err != nil || absent.Runtime.PasswordChange != nil || absent.MinimumPasswordCharacters != 0 || absent.PasswordBlocklistFile != "" {
		t.Fatal("unconfigured API implicitly enabled password change")
	}
	c, err := Parse(passwordChangeConfigBody(t, passwordChangeWireFixture(), 0))
	if err != nil || c.Runtime.PasswordChange == nil || c.Runtime.PasswordChange.Acceptance != nil || c.MinimumPasswordCharacters != 15 ||
		c.PasswordBlocklistFile != "/config/weak-passwords.json" {
		t.Fatal("explicit password-change configuration not retained")
	}
	limits := c.Runtime.PasswordChange.Limits
	if limits.Global.Count != 11 || limits.Global.Window != time.Minute || limits.Source.Count != 7 || limits.Source.Window != 2*time.Minute ||
		limits.Subject.Count != 3 || limits.Subject.Window != 3*time.Minute {
		t.Fatal("independent change quotas were defaulted or mixed")
	}
	maximal := passwordChangeWireFixture()
	maximal["minimum_characters"] = 64
	if _, err := Parse(passwordChangeConfigBody(t, maximal, 256)); err != nil {
		t.Fatal("explicit maximum minimum with sufficient byte budget rejected")
	}
}

func TestPasswordChangeConfigurationRejectsMissingInvalidAndConflictingInputs(t *testing.T) {
	for _, kind := range []string{"null", "empty", "missing minimum", "missing file", "missing global", "missing source", "missing subject", "below floor", "minimum overflow", "capacity conflict",
		"fractional minimum", "string minimum", "integer overflow", "zero count", "negative count", "fractional count", "count overflow", "zero window", "submillisecond window", "oversize window", "missing window", "null bucket", "unknown field", "duplicate field"} {
		t.Run(kind, func(t *testing.T) {
			change := passwordChangeWireFixture()
			var value any = change
			switch kind {
			case "null":
				value = nil
			case "empty":
				value = map[string]any{}
			case "missing minimum":
				delete(change, "minimum_characters")
			case "missing file":
				delete(change, "blocklist_file")
			case "missing global":
				delete(change, "global")
			case "missing source":
				delete(change, "source")
			case "missing subject":
				delete(change, "subject")
			case "below floor":
				change["minimum_characters"] = 14
			case "minimum overflow":
				change["minimum_characters"] = 65
			case "capacity conflict":
				change["minimum_characters"] = 33
			case "fractional minimum":
				change["minimum_characters"] = 15.5
			case "string minimum":
				change["minimum_characters"] = "15"
			case "integer overflow":
				change["minimum_characters"] = json.Number("9223372036854775808")
			case "zero count":
				change["global"].(map[string]any)["count"] = 0
			case "negative count":
				change["source"].(map[string]any)["count"] = -1
			case "fractional count":
				change["subject"].(map[string]any)["count"] = 1.5
			case "count overflow":
				change["subject"].(map[string]any)["count"] = 1000001
			case "zero window":
				change["global"].(map[string]any)["window"] = "0s"
			case "submillisecond window":
				change["source"].(map[string]any)["window"] = "1500us"
			case "oversize window":
				change["subject"].(map[string]any)["window"] = "25h"
			case "missing window":
				delete(change["subject"].(map[string]any), "window")
			case "null bucket":
				change["subject"] = nil
			case "unknown field":
				change["env"] = "UNTRUSTED_ENV"
			}
			body := passwordChangeConfigBody(t, value, 0)
			if kind == "duplicate field" {
				body = []byte(strings.Replace(string(body), `"minimum_characters":15`, `"minimum_characters":15,"minimum_char\u0061cters":15`, 1))
			}
			reject(t, body)
		})
	}
	for _, path := range []string{"", "relative.json", "/config/../weak-passwords.json", "/config//weak-passwords.json", "/config/weak\x00passwords.json", "/" + strings.Repeat("a", 4096)} {
		change := passwordChangeWireFixture()
		change["blocklist_file"] = path
		reject(t, passwordChangeConfigBody(t, change, 0))
	}
	change := passwordChangeWireFixture()
	change["minimum_characters"] = 16
	if _, err := Parse(passwordChangeConfigBody(t, change, 63)); err != ErrConfig {
		t.Fatal("byte budget below four bytes per minimum code point accepted")
	}
}

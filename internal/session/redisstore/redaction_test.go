package redisstore

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestSensitiveValueFormattingAndJSON(t *testing.T) {
	store, config, attempt := sealingFixture(t)
	secrets, err := parseLoginSecrets([]byte(syntheticSecrets()))
	if err != nil {
		t.Fatal(err)
	}
	login, err := NewLoginFromSecrets(store, config.Origin, config.Window, secrets)
	if err != nil {
		t.Fatal(err)
	}
	login.admission, err = newLoginAdmission(store, secrets, unitLoginLimits())
	if err != nil {
		t.Fatal(err)
	}
	continuation, err := login.seal(attempt)
	if err != nil {
		t.Fatal(err)
	}
	credential := attempt.credential
	rawCredential, rawContinuation := credential.Value(), continuation.Value()
	key := config.Keys[config.CurrentKey]
	formats := []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%f", "%x", "%X", "%b", "%o", "%e", "%c", "%U", "%t", "%020.3v"}
	for _, tc := range []struct {
		name, label, json      string
		value, pointer, absent any
	}{
		{"credential", "[redacted session credential]", `{}`, credential, &credential, (*Credential)(nil)},
		{"attempt", "[redacted session attempt]", `{}`, *attempt, attempt, (*Attempt)(nil)},
		{"continuation", "[redacted login continuation]", `{}`, continuation, &continuation, (*Continuation)(nil)},
		{"secrets", "[redacted login secrets]", `{}`, *secrets, secrets, (*LoginSecrets)(nil)},
		{"configuration", "[redacted login sealing configuration]", `"[redacted login sealing configuration]"`, config, &config, (*LoginConfig)(nil)},
		{"manager", "[redacted login manager]", `{}`, *login, login, (*Login)(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range []any{tc.value, tc.pointer} {
				for _, format := range formats {
					if fmt.Sprintf(format, v) != tc.label {
						t.Errorf("direct sensitive formatting was not redacted for verb %s", format)
					}
				}
				body, err := json.Marshal(v)
				if err != nil || string(body) != tc.json {
					t.Error("sensitive JSON exposed fields or changed its redacted shape")
				}
			}
			for _, format := range formats {
				if fmt.Sprintf(format, tc.absent) != "<nil>" {
					t.Fatalf("typed nil formatting changed for verb %s", format)
				}
			}
			body, err := json.Marshal(tc.absent)
			if err != nil || string(body) != "null" {
				t.Fatal("typed nil JSON changed")
			}
		})
	}
	// Presentation must not consume or mutate the actual protocol capabilities.
	parsedCredential, err := ParseCredential(rawCredential)
	if err != nil || parsedCredential != credential {
		t.Fatal("formatting changed explicit credential roundtrip")
	}
	parsedContinuation, err := ParseContinuation(rawContinuation)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := login.open(parsedContinuation)
	if err != nil || !reflect.DeepEqual(opened, attempt) || continuation.Value() != rawContinuation || credential.Value() != rawCredential || config.Keys[config.CurrentKey] != key {
		t.Fatal("redaction mutated key material or sealed protocol data")
	}
}

func TestLoginConfigJSONInStandardContainers(t *testing.T) {
	_, config, _ := sealingFixture(t)
	for _, value := range []any{
		[]LoginConfig{config},
		map[string]LoginConfig{"config": config},
		struct{ Config LoginConfig }{config},
	} {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatal(err)
		}
		switch result := decoded.(type) {
		case []any:
			if len(result) != 1 || result[0] != "[redacted login sealing configuration]" {
				t.Fatal("slice bypassed config JSON redaction")
			}
		case map[string]any:
			if len(result) != 1 || (result["config"] != "[redacted login sealing configuration]" && result["Config"] != "[redacted login sealing configuration]") {
				t.Fatal("container bypassed config JSON redaction")
			}
		default:
			t.Fatal("unexpected encoded config container")
		}
	}
}

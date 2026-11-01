package redisstore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func syntheticSecret(value byte) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat(string([]byte{value}), 32)))
}

func syntheticSecrets() string {
	return `{"version":1,"current":"active","sealing":{"active":"` + syntheticSecret(1) + `"},"limiting":"` + syntheticSecret(2) + `"}`
}

func TestLoginSecretsStrictSchema(t *testing.T) {
	valid := syntheticSecrets()
	if s, err := parseLoginSecrets([]byte(valid)); err != nil || s.current != "active" {
		t.Fatal("valid synthetic secrets rejected")
	}
	cases := map[string]string{
		"missing":             strings.Replace(valid, `"version":1,`, "", 1),
		"unknown":             strings.Replace(valid, `"version":1`, `"version":1,"extra":true`, 1),
		"duplicate":           strings.Replace(valid, `"version":1`, `"version":1,"version":1`, 1),
		"escaped duplicate":   strings.Replace(valid, `"version":1`, `"version":1,"vers\u0069on":1`, 1),
		"nested duplicate":    strings.Replace(valid, `"sealing":{`, `"sealing":{"active":"`+syntheticSecret(3)+`",`, 1),
		"null":                strings.Replace(valid, `"current":"active"`, `"current":null`, 1),
		"null keyset":         strings.Replace(valid, `{"active":"`+syntheticSecret(1)+`"}`, `null`, 1),
		"absent current":      strings.Replace(valid, `"current":"active"`, `"current":"absent"`, 1),
		"empty keyset":        strings.Replace(valid, `{"active":"`+syntheticSecret(1)+`"}`, `{}`, 1),
		"bad ID":              strings.ReplaceAll(valid, `active`, `bad.key`),
		"decimal version":     strings.Replace(valid, `"version":1`, `"version":1.0`, 1),
		"exponent version":    strings.Replace(valid, `"version":1`, `"version":1e0`, 1),
		"string version":      strings.Replace(valid, `"version":1`, `"version":"1"`, 1),
		"padding":             strings.Replace(valid, syntheticSecret(1), syntheticSecret(1)+"=", 1),
		"short key":           strings.Replace(valid, syntheticSecret(1), "AAAA", 1),
		"zero key":            strings.Replace(valid, syntheticSecret(1), syntheticSecret(0), 1),
		"zero limiting":       strings.Replace(valid, syntheticSecret(2), syntheticSecret(0), 1),
		"reused purpose":      strings.Replace(valid, syntheticSecret(2), syntheticSecret(1), 1),
		"reused sealing":      strings.Replace(valid, `"sealing":{`, `"sealing":{"old":"`+syntheticSecret(1)+`",`, 1),
		"too many":            strings.Replace(valid, `"sealing":{`, `"sealing":{"b":"`+syntheticSecret(3)+`","c":"`+syntheticSecret(4)+`","d":"`+syntheticSecret(5)+`","e":"`+syntheticSecret(6)+`",`, 1),
		"noncanonical base64": strings.Replace(valid, syntheticSecret(1), syntheticSecret(1)[:42]+"F", 1),
		"trailing":            valid + `{}`,
		"array":               `[]`,
		"unicode":             strings.Replace(valid, `active`, `\ud800`, 1),
		"invalid UTF8":        valid + string([]byte{255}),
		"oversize":            valid + strings.Repeat(" ", maxLoginSecretsBytes),
		"empty":               "",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if s, err := parseLoginSecrets([]byte(body)); s != nil || err != ErrLoginSecrets || errors.Unwrap(err) != nil {
				t.Fatal("malformed secrets did not fail with a redacted stable error")
			}
		})
	}
}

func TestLoginSecretsSnapshotAndRotation(t *testing.T) {
	store, cfg, attempt := sealingFixture(t)
	old, err := parseLoginSecrets([]byte(syntheticSecrets()))
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewLoginFromSecrets(store, cfg.Origin, time.Minute, old)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := first.seal(attempt)
	if err != nil {
		t.Fatal(err)
	}
	rotated := strings.Replace(syntheticSecrets(), `"current":"active"`, `"current":"next"`, 1)
	rotated = strings.Replace(rotated, `"sealing":{`, `"sealing":{"next":"`+syntheticSecret(3)+`",`, 1)
	next, err := parseLoginSecrets([]byte(rotated))
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewLoginFromSecrets(store, cfg.Origin, time.Minute, next)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.open(candidate); err != nil {
		t.Fatal("retained key did not restore old candidate")
	}
	fresh, err := second.seal(attempt)
	if err != nil || !strings.HasPrefix(fresh.Value(), "1.next.") || old.limiting != next.limiting {
		t.Fatal("rotation changed limiting material or failed to select new sealing key")
	}
	for _, secrets := range []any{old, *old, next, *next} {
		formatted := fmt.Sprintf("%v %+v %#v", secrets, secrets, secrets)
		encoded, err := json.Marshal(secrets)
		if err != nil || string(encoded) != "{}" || formatted != strings.Repeat("[redacted login secrets] ", 2)+"[redacted login secrets]" {
			t.Fatal("opaque secrets exposed private fields")
		}
	}
	var absent *LoginSecrets
	if text := fmt.Sprintf("%v %+v %#v", absent, absent, absent); text != "<nil> <nil> <nil>" {
		t.Fatal("nil snapshot formatting exposed data")
	}
	if l, err := NewLoginFromSecrets(store, cfg.Origin, time.Minute, nil); l != nil || err != ErrLoginSecrets {
		t.Fatal("nil secrets accepted")
	}
	if l, err := NewLoginFromSecrets(store, "http://invalid.example", time.Minute, old); l != nil || err != ErrInvalid {
		t.Fatal("origin checks bypassed")
	}
}

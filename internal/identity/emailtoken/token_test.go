package emailtoken_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
)

func TestTokenEncodingAndPurpose(t *testing.T) {
	token := newToken(t)
	link := token.LinkValue()
	if len(link) != 43 || strings.Contains(fmt.Sprintf("%v %#v", token, token), link) {
		t.Fatal("unexpected token encoding or diagnostic exposure")
	}
	parsed, err := emailtoken.Parse(link)
	if err != nil || parsed != token || newToken(t) == token {
		t.Fatal("unexpected token round trip or repeated credential")
	}
	a, errA := parsed.Digest(emailtoken.Activation)
	b, errB := parsed.Digest(emailtoken.Recovery)
	if errA != nil || errB != nil || a == b || a == [32]byte{} || b == [32]byte{} {
		t.Fatal("credential digest did not separate purposes")
	}
	for _, bad := range []string{"", link + "=", link[:42] + "!", "\n" + link[1:], base64.RawURLEncoding.EncodeToString(make([]byte, 32))} {
		if _, err := emailtoken.Parse(bad); err == nil {
			t.Fatal("accepted malformed credential")
		}
	}
}

func TestPayloadRotationAndBinding(t *testing.T) {
	now := time.Date(2026, 11, 13, 0, 0, 0, 123000, time.UTC)
	scope := emailtoken.Scope{
		PayloadID: strings.Repeat("a", 32), SubjectID: strings.Repeat("b", 32), RecoveryID: strings.Repeat("c", 32),
		AuthGeneration: 1,
		Recipient:      email(t, "Test+one@example.invalid"), Purpose: emailtoken.Recovery,
		IssuedAt: now, SendUntil: now.Add(10 * time.Minute), TokenExpiry: now.Add(30 * time.Minute),
	}
	token := newToken(t)
	old := protector(t, "old", map[string][32]byte{"old": {1}})
	active := protector(t, "new", map[string][32]byte{"old": {1}, "new": {2}})
	retired := protector(t, "new", map[string][32]byte{"new": {2}})
	before, err := old.Seal(now, scope, token)
	if err != nil {
		t.Fatal(err)
	}
	after, err := active.Seal(now, scope, token)
	if err != nil || after.KeyID != "new" {
		t.Fatal("rotation did not seal with the new key")
	}
	for _, tc := range []struct {
		value, pointer any
		label, json    string
	}{
		{token, &token, "[redacted email token]", "{}"},
		{after, &after, "[redacted email payload]", `"[redacted email payload]"`},
		{*active, active, "[redacted email payload protector]", "{}"},
	} {
		for _, value := range []any{tc.value, tc.pointer} {
			for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%x", "%b", "%U", "%020.3v"} {
				if fmt.Sprintf(verb, value) != tc.label {
					t.Fatalf("direct format %s did not redact the object", verb)
				}
			}
			body, err := json.Marshal(value)
			if err != nil || string(body) != tc.json {
				t.Fatal("JSON exposed sensitive material")
			}
		}
	}
	repeat, err := active.Seal(now, scope, token)
	if err != nil || bytes.Equal(after.Ciphertext, repeat.Ciphertext) {
		t.Fatal("unexpected nonce reuse or seal failure")
	}
	for _, payload := range []emailtoken.Payload{before, after} {
		got, err := active.Open(scope.SendUntil.Add(-time.Nanosecond), scope, payload)
		if err != nil || got != token {
			t.Fatal("key overlap did not recover the original token")
		}
	}
	if _, err := retired.Open(now, scope, before); err == nil {
		t.Fatal("opened a retired key")
	}
	for _, at := range []time.Time{now.Add(-time.Nanosecond), scope.SendUntil, scope.TokenExpiry} {
		if _, err := active.Open(at, scope, after); err == nil {
			t.Fatal("opened ciphertext outside the send window")
		}
	}
	for _, change := range []func(*emailtoken.Scope){
		func(s *emailtoken.Scope) { s.PayloadID = strings.Repeat("d", 32) },
		func(s *emailtoken.Scope) { s.SubjectID = strings.Repeat("d", 32) },
		func(s *emailtoken.Scope) { s.RecoveryID = strings.Repeat("d", 32) },
		func(s *emailtoken.Scope) { s.AuthGeneration++ },
		func(s *emailtoken.Scope) { s.Recipient = email(t, "test+one@example.invalid") },
		func(s *emailtoken.Scope) { s.Purpose = emailtoken.Activation },
		func(s *emailtoken.Scope) { s.TokenExpiry = s.TokenExpiry.Add(-time.Microsecond) },
		func(s *emailtoken.Scope) { s.SendUntil = s.SendUntil.Add(-time.Microsecond) },
		func(s *emailtoken.Scope) {
			s.IssuedAt = s.IssuedAt.Add(-time.Microsecond)
			s.SendUntil = s.SendUntil.Add(-time.Microsecond)
		},
	} {
		changed := scope
		change(&changed)
		if _, err := active.Open(now, changed, after); err == nil {
			t.Fatal("opened payload with a different bound field")
		}
	}
	tampered := emailtoken.Payload{KeyID: "new", Ciphertext: bytes.Clone(after.Ciphertext)}
	tampered.Ciphertext[len(tampered.Ciphertext)-1] ^= 1
	if _, err := active.Open(now, scope, tampered); err == nil {
		t.Fatal("accepted tampered ciphertext")
	}
	scope.SendUntil = now.Add(10*time.Minute + time.Microsecond)
	if _, err := active.Seal(now, scope, token); err == nil {
		t.Fatal("sealed an overlong send window")
	}
	scope.SendUntil = now.Add(10 * time.Minute)
	for _, change := range []func(*emailtoken.Scope){
		func(s *emailtoken.Scope) { s.Purpose = "unknown" },
		func(s *emailtoken.Scope) { s.SendUntil = s.SendUntil.Add(-time.Nanosecond) },
		func(s *emailtoken.Scope) { s.TokenExpiry = s.TokenExpiry.Add(time.Microsecond) },
		func(s *emailtoken.Scope) { s.TokenExpiry = s.SendUntil.Add(-time.Microsecond) },
	} {
		bad := scope
		change(&bad)
		if _, err := active.Seal(now, bad, token); err == nil {
			t.Fatal("sealed invalid persisted metadata")
		}
	}
}

func TestProtectorRejectsAmbiguousOrInvalidKeyRings(t *testing.T) {
	for _, keys := range []map[string][32]byte{
		nil, {"other": {1}}, {"new": {}}, {"new": {1}, "old": {1}},
		{"new": {1}, "BAD": {2}}, {"new": {1}, "a": {2}, "b": {3}, "c": {4}, "d": {5}},
	} {
		if _, err := emailtoken.NewProtector("new", keys); err == nil {
			t.Fatal("accepted invalid key ring")
		}
	}
}

func newToken(t *testing.T) emailtoken.Token {
	t.Helper()
	token, err := emailtoken.New()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func email(t *testing.T, value string) identity.Email {
	t.Helper()
	e, err := identity.ParseEmail(value)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func protector(t *testing.T, id string, keys map[string][32]byte) *emailtoken.Protector {
	t.Helper()
	p, err := emailtoken.NewProtector(id, keys)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

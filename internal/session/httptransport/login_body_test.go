package httptransport

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginBodyPreservesAndBoundsStrings(t *testing.T) {
	for _, tc := range []struct {
		name, body, media string
		status            int
		secret            string
	}{
		{"valid", `{"email":"Person@example.invalid","password":"密\\码😀"}`, "application/json", 200, "密\\码😀"},
		{"duplicate", `{"email":"a@example.invalid","password":"a","password":"b"}`, "application/json", 400, ""},
		{"unknown", `{"email":"a@example.invalid","password":"a","remember":true}`, "application/json", 400, ""},
		{"missing", `{"email":"a@example.invalid"}`, "application/json", 400, ""},
		{"null", `{"email":"a@example.invalid","password":null}`, "application/json", 400, ""},
		{"array", `[]`, "application/json", 400, ""},
		{"trailing", `{"email":"a@example.invalid","password":"a"}{}`, "application/json", 400, ""},
		{"lone surrogate", `{"email":"a@example.invalid","password":"\ud800"}`, "application/json", 400, ""},
		{"invalid UTF8", `{"email":"a@example.invalid","password":"` + string([]byte{255}) + `"}`, "application/json", 400, ""},
		{"bad email", `{"email":"name","password":"a"}`, "application/json", 400, ""},
		{"empty secret", `{"email":"a@example.invalid","password":""}`, "application/json", 400, ""},
		{"media", `{}`, "text/plain", 415, ""},
		{"large", strings.Repeat(" ", 4097), "application/json", 413, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "https://example.invalid", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.media)
			w := httptest.NewRecorder()
			email, secret, err := loginBody(w, r)
			if tc.status == 200 {
				if err != nil || email.Key() != "person@example.invalid" || string(secret) != tc.secret {
					t.Fatal("decoded credential strings changed")
				}
			} else if err == nil || w.Code != tc.status || len(secret) != 0 || email.Valid() {
				t.Fatal("invalid login body accepted or misclassified")
			}
			clear(secret)
		})
	}
}

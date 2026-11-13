//go:build linux || darwin

package emailtoken_test

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
)

func TestLoadProtectorRotation(t *testing.T) {
	path := keyFilePath(t)
	old, newKey := encodedKey(1), encodedKey(2)
	load := func(current, keys string) *emailtoken.Protector {
		t.Helper()
		writeKeyFile(t, path, keyFile(current, keys))
		p, err := emailtoken.LoadProtector(path)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	now := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	scope := emailtoken.Scope{
		PayloadID: strings.Repeat("a", 32), SubjectID: strings.Repeat("b", 32), RecoveryID: strings.Repeat("c", 32),
		AuthGeneration: 1, Recipient: email(t, "recipient@example.invalid"), Purpose: emailtoken.Activation,
		IssuedAt: now, SendUntil: now.Add(time.Minute), TokenExpiry: now.Add(time.Hour),
	}
	token := newToken(t)
	initial := load("old", fmt.Sprintf(`"old":%q`, old))
	pending, err := initial.Seal(now, scope, token)
	if err != nil {
		t.Fatal(err)
	}
	overlap := load("new", fmt.Sprintf(`"old":%q,"new":%q`, old, newKey))
	retry, err := overlap.Open(now, scope, pending)
	if err != nil || retry != token {
		t.Fatal("overlap could not reopen an existing job")
	}
	fresh, err := overlap.Seal(now, scope, token)
	if err != nil || fresh.KeyID != "new" {
		t.Fatal("overlap did not seal under the active key")
	}
	retired := load("new", fmt.Sprintf(`"new":%q`, newKey))
	if _, err := retired.Open(now, scope, pending); err != emailtoken.ErrInvalid {
		t.Fatal("retired key was still accepted")
	}
	if got, err := retired.Open(now, scope, fresh); err != nil || got != token {
		t.Fatal("retirement broke jobs sealed with the active key")
	}
}

func TestLoadProtectorRejectsInvalidFiles(t *testing.T) {
	path := keyFilePath(t)
	k := encodedKey(1)
	valid := keyFile("one", fmt.Sprintf(`"one":%q`, k))
	rejected := func(path string) {
		t.Helper()
		if p, err := emailtoken.LoadProtector(path); p != nil || err != emailtoken.ErrKeyFile {
			t.Fatal("expected the same redacted error without a protector")
		}
	}
	rejected(path)
	keys := fmt.Sprintf(`"one":%q,"two":%q,"three":%q,"four":%q`, k, encodedKey(2), encodedKey(3), encodedKey(4))
	writeKeyFile(t, path, keyFile("one", keys))
	if p, err := emailtoken.LoadProtector(path); err != nil || p.CurrentKeyID() != "one" {
		t.Fatal("expected a valid protector at the four-key limit")
	}
	for _, tc := range []struct{ name, body string }{
		{"oversize", valid + strings.Repeat(" ", 4096)},
		{"too many keys", keyFile("one", keys+fmt.Sprintf(`,"five":%q`, encodedKey(5)))},
		{"unknown version", strings.Replace(valid, ".v1", ".v2", 1)},
		{"unknown field", valid[:len(valid)-1] + `,"extra":true}`},
		{"duplicate field", valid[:len(valid)-1] + `,"\u0063urrent":"one"}`},
		{"trailing data", valid + `{}`},
		{"wrong document type", `[]`},
		{"invalid unicode", strings.Replace(valid, "one", "\xff", 1)},
		{"missing active key", keyFile("absent", fmt.Sprintf(`"one":%q`, k))},
		{"invalid key ID", keyFile("One", fmt.Sprintf(`"One":%q`, k))},
		{"duplicate ID", keyFile("one", fmt.Sprintf(`"one":%q,"one":%q`, k, encodedKey(2)))},
		{"duplicate key bytes", keyFile("one", fmt.Sprintf(`"one":%q,"two":%q`, k, k))},
		{"zero key", keyFile("one", fmt.Sprintf(`"one":%q`, encodedKey(0)))},
		{"padded key", keyFile("one", fmt.Sprintf(`"one":%q`, k+"="))},
		{"alternate encoding", keyFile("one", fmt.Sprintf(`"one":%q`, k[:42]+"B"))},
		{"wrong key type", keyFile("one", `"one":[]`)},
		{"login secrets document", `{"version":1,"current":"one","sealing":{"one":` + fmt.Sprintf("%q", k) + `}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeKeyFile(t, path, tc.body)
			rejected(path)
		})
	}
	writeKeyFile(t, path, valid)
	link := path + "-link"
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	rejected(link)
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	rejected(path)
}

func keyFilePath(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "mail-keys.json")
}

func writeKeyFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func encodedKey(first byte) string {
	key := [32]byte{first}
	return base64.RawURLEncoding.EncodeToString(key[:])
}

func keyFile(current, keys string) string {
	return fmt.Sprintf(`{"format":"oj.email-payload-keys.v1","current":%q,"keys":{%s}}`, current, keys)
}

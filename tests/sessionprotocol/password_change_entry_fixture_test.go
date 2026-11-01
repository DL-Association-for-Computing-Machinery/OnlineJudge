//go:build integration

package sessionprotocol_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

func enableEntryPasswordChange(t *testing.T, f *fixture, c *sessionruntime.Config, limits protocol.PasswordChangeLimits, provision bool) string {
	t.Helper()
	blocked := sha256.Sum256([]byte("synthetic blocked password"))
	rules, err := password.NewAcceptance(15, c.PasswordLimits.MaxPasswordBytes, [][32]byte{blocked})
	must(t, err)
	c.PasswordChange = &sessionruntime.PasswordChangeConfig{Limits: limits, Acceptance: rules}
	secrets, err := protocol.LoadLoginSecrets(c.SecretsFile)
	must(t, err)
	if provision {
		must(t, protocol.InitializePasswordChangeLimits(f.ctx, f.store(t, c.Namespace, 0, protocol.OrdinaryPolicy()), secrets, limits, rules))
	}
	body, err := json.Marshal(map[string]any{"version": 1, "sha256": []string{hex.EncodeToString(blocked[:])}})
	must(t, err)
	path := filepath.Join(t.TempDir(), "weak-password-digests.json")
	must(t, os.WriteFile(path, body, 0600))
	return entryPath(t, path)
}

func changeBody(t *testing.T, current, replacement string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"current_password": current, "new_password": replacement})
	must(t, err)
	return string(body)
}

func entryLoginBody(t *testing.T, p identity.Personal, secret string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"email": p.Email.Original(), "password": secret})
	must(t, err)
	return string(body)
}

func entryCredential(t *testing.T, e *loginEndpoint, p identity.Personal, secret string) protocol.Credential {
	t.Helper()
	begin := loginCall(e, "/api/v1/session/login/begin", entryLoginBody(t, p, secret), nil, nil)
	loginStatus(t, begin, 202)
	if len(begin.cookies) != 1 {
		t.Fatal("begin did not return exactly one pending cookie")
	}
	return completedCredential(t, loginCall(e, "/api/v1/session/login/complete", "", begin.cookies, nil))
}

func credentialCookies(credential protocol.Credential) []*http.Cookie {
	return []*http.Cookie{{Name: "__Host-oj_session", Value: credential.Value()}}
}

func requireChangeFailure(t *testing.T, response loginResponse, status int, code string) {
	t.Helper()
	loginStatus(t, response, status)
	if len(response.headers.Values("Set-Cookie")) != 0 {
		t.Fatal("failed change published Cookie cleanup or a credential")
	}
	var problem struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(response.body, &problem) != nil || problem.Code != code {
		t.Fatal("password change failure had wrong stable code")
	}
}

func requireChangeClearedCookies(t *testing.T, response loginResponse) {
	t.Helper()
	loginStatus(t, response, 204)
	if len(response.body) != 0 || len(response.cookies) != 2 {
		t.Fatal("confirmed change did not clear both cookies with no body")
	}
	seen := map[string]bool{}
	for _, cookie := range response.cookies {
		if (cookie.Name != "__Host-oj_session" && cookie.Name != "__Host-oj_login") || seen[cookie.Name] || cookie.Value != "" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.MaxAge != -1 {
			t.Fatal("confirmed change Cookie cleanup contract changed")
		}
		seen[cookie.Name] = true
	}
}

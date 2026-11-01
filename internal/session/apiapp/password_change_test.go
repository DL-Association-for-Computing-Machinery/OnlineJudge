package apiapp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	passwordpolicy "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/apiconfig"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

func passwordBlocklistFixture(t *testing.T, mode os.FileMode) string {
	t.Helper()
	digest := sha256.Sum256([]byte("synthetic common password"))
	body, err := json.Marshal(map[string]any{"version": 1, "sha256": []string{hex.EncodeToString(digest[:])}})
	if err != nil {
		t.Fatal("blocklist fixture encoding failed")
	}
	path := filepath.Join(t.TempDir(), "weak-passwords.json")
	if os.WriteFile(path, body, mode) != nil {
		t.Fatal("blocklist fixture write failed")
	}
	if os.Chmod(path, mode) != nil {
		t.Fatal("blocklist fixture mode setup failed")
	}
	return canonical(t, path)
}

func passwordAcceptanceConfig(path string) apiconfig.Config {
	c := apiconfig.Config{PasswordBlocklistFile: path, MinimumPasswordCharacters: 15}
	c.Runtime.PasswordChange = &sessionruntime.PasswordChangeConfig{}
	c.Runtime.PasswordLimits.MaxPasswordBytes = 128
	return c
}

func TestPasswordAcceptanceLoadsOnlyExplicitCapabilityAndPublicDigestFile(t *testing.T) {
	for _, mode := range []os.FileMode{0400, 0444, 0600, 0644} {
		path := passwordBlocklistFixture(t, mode)
		c := passwordAcceptanceConfig(path)
		if loadPasswordAcceptance(&c) != nil || c.Runtime.PasswordChange.Acceptance == nil {
			t.Fatal("safe nonsecret blocklist file was not loaded")
		}
		rules := c.Runtime.PasswordChange.Acceptance
		if rules.Fingerprint() == ([32]byte{}) || rules.MaxBytes() != 128 || rules.Check([]byte("synthetic common password")) != passwordpolicy.ErrWeakPassword ||
			rules.Check([]byte("short")) != passwordpolicy.ErrNewPasswordLength || rules.Check([]byte("synthetic stronger replacement")) != nil {
			t.Fatal("loaded blocklist changed explicit policy")
		}
	}
	c := apiconfig.Config{PasswordBlocklistFile: "/never-read-when-disabled/synthetic-input", MinimumPasswordCharacters: 15}
	if loadPasswordAcceptance(&c) != nil || c.Runtime.PasswordChange != nil {
		t.Fatal("disabled capability read material or enabled change")
	}
}

func TestPasswordAcceptanceRejectsBadFileMaterialWithoutRawDiagnostics(t *testing.T) {
	for _, kind := range []string{"missing", "malformed", "empty", "duplicate digest", "unknown key", "wrong version", "null", "oversize", "group writable", "world writable", "executable", "symlink", "directory", "minimum conflict", "byte budget conflict"} {
		t.Run(kind, func(t *testing.T) {
			path := passwordBlocklistFixture(t, 0600)
			c := passwordAcceptanceConfig(path)
			switch kind {
			case "missing":
				c.PasswordBlocklistFile = filepath.Join(filepath.Dir(path), "missing-synthetic-input.json")
			case "malformed":
				if os.WriteFile(path, []byte(`{"synthetic-input":`), 0600) != nil {
					t.Fatal("fixture mutation failed")
				}
			case "empty":
				if os.WriteFile(path, nil, 0600) != nil {
					t.Fatal("fixture mutation failed")
				}
			case "duplicate digest":
				digest := sha256.Sum256([]byte("synthetic common password"))
				encoded := hex.EncodeToString(digest[:])
				if os.WriteFile(path, []byte(`{"version":1,"sha256":["`+encoded+`","`+encoded+`"]}`), 0600) != nil {
					t.Fatal("fixture mutation failed")
				}
			case "unknown key":
				if os.WriteFile(path, []byte(`{"version":1,"sha256":[],"synthetic-input":1}`), 0600) != nil {
					t.Fatal("fixture mutation failed")
				}
			case "wrong version":
				if os.WriteFile(path, []byte(`{"version":2,"sha256":[]}`), 0600) != nil {
					t.Fatal("fixture mutation failed")
				}
			case "null":
				if os.WriteFile(path, []byte(`null`), 0600) != nil {
					t.Fatal("fixture mutation failed")
				}
			case "oversize":
				if os.WriteFile(path, []byte(strings.Repeat(" ", (1<<20)+1)), 0600) != nil {
					t.Fatal("fixture mutation failed")
				}
			case "group writable":
				if os.Chmod(path, 0660) != nil {
					t.Fatal("fixture mode mutation failed")
				}
			case "world writable":
				if os.Chmod(path, 0666) != nil {
					t.Fatal("fixture mode mutation failed")
				}
			case "executable":
				if os.Chmod(path, 0700) != nil {
					t.Fatal("fixture mode mutation failed")
				}
			case "symlink":
				link := filepath.Join(filepath.Dir(path), "synthetic-link.json")
				if os.Symlink(path, link) != nil {
					t.Fatal("fixture symlink failed")
				}
				c.PasswordBlocklistFile = link
			case "directory":
				c.PasswordBlocklistFile = filepath.Dir(path)
			case "minimum conflict":
				c.MinimumPasswordCharacters = 0
			case "byte budget conflict":
				c.Runtime.PasswordLimits.MaxPasswordBytes = 59
			}
			err := loadPasswordAcceptance(&c)
			if err != ErrConfiguration || err.Error() != "session API configuration unavailable or invalid" || strings.Contains(err.Error(), c.PasswordBlocklistFile) ||
				strings.Contains(err.Error(), "synthetic-input") || c.Runtime.PasswordChange.Acceptance != nil {
				t.Fatal("invalid blocklist loaded or file/input details escaped")
			}
		})
	}
}

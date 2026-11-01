//go:build integration

package sessionprotocol_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func loadedLoginManager(t *testing.T, s *protocol.Store, window time.Duration, key, limiting [32]byte) *protocol.Login {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	must(t, err)
	path := filepath.Join(dir, "synthetic-login.json")
	body, err := json.Marshal(map[string]any{"version": 1, "current": "fixture", "sealing": map[string]string{"fixture": base64.RawURLEncoding.EncodeToString(key[:])}, "limiting": base64.RawURLEncoding.EncodeToString(limiting[:])})
	must(t, err)
	must(t, os.WriteFile(path, body, 0600))
	secrets, err := protocol.LoadLoginSecrets(path)
	must(t, err)
	l, err := protocol.NewLoginFromSecrets(s, loginOrigin, window, secrets)
	must(t, err)
	return l
}

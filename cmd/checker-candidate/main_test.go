package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCandidateCommandRequiresSettingsAndPinnedLock(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "source-lock.json")
	if err := os.WriteFile(lock, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	flags := []string{
		"--bundle", dir, "--source-id", "library-checker:aplusb", "--source-lock", lock,
		"--expected-plan", strings.Repeat("a", 64), "--checker", "lc-wcmp-tokens-v1", "--bytes", "32", "--token-bytes", "12",
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{flags[:len(flags)-2], "explicit checker settings"},
		{flags, "source lock admission required"},
	} {
		var out, diagnostics bytes.Buffer
		if err := run(t.Context(), test.args, &out, &diagnostics); err == nil || !strings.Contains(err.Error(), test.want) || out.Len() != 0 {
			t.Fatalf("expected %q and no candidate: %v, %q", test.want, err, out.String())
		}
	}
}

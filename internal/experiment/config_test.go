package experiment_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func TestExperimentRejectsPublicBindingsAndReadableTokens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 32)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, addr := range []string{"0.0.0.0:8080", ":8080", "localhost:8080", "[::]:8080", "192.0.2.1:8080", "invalid"} {
		if _, err := experiment.LoadToken(addr, path); err == nil {
			t.Fatalf("accepted binding %s", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:8080", "[::1]:8080"} {
		if _, err := experiment.LoadToken(addr, path); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := experiment.LoadToken("127.0.0.1:8080", path); err == nil {
		t.Fatal("readable token accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("short"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := experiment.LoadToken("127.0.0.1:8080", path); err == nil {
		t.Fatal("short token accepted")
	}
}

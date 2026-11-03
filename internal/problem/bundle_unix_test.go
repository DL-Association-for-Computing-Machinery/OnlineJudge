//go:build linux || darwin

package problem

import (
	"os"
	"syscall"
	"testing"
)

func makeBundleFIFO(t *testing.T, target string) {
	t.Helper()
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(target, 0600); err != nil {
		t.Fatal(err)
	}
}

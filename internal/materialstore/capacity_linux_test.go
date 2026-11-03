package materialstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestBoundedTmpfsRealWriteExhaustion(t *testing.T) {
	if os.Getenv("OJ_MATERIALSTORE_CAPACITY") != "1" {
		t.Skip("requires explicitly bounded, empty Docker tmpfs")
	}
	const root = "/oj-materialstore-capacity"
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(root, &filesystem); err != nil {
		t.Fatal(err)
	}
	// Refuse to fill an ordinary filesystem or a larger shared mount.
	if filesystem.Type != 0x01021994 || filesystem.Blocks*uint64(filesystem.Bsize) > 1<<20 {
		t.Fatal("dedicated tmpfs <=1MiB required")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("capacity fixture must start empty", err)
	}
	s, err := OpenLocal(Config{Root: root, MaxObjectBytes: 2 << 20, MaxInFlight: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	data := strings.Repeat("x", 2<<20)
	object := identity(data)
	if _, err := s.Put(context.Background(), object, strings.NewReader(data)); !errors.Is(err, syscall.ENOSPC) || !errors.Is(err, ErrUnavailable) {
		t.Fatal("real write exhaustion not observed", err)
	}
	if _, err := os.Stat(filepath.Join(root, object.SHA256)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed large object published")
	}
	if _, err := s.Put(context.Background(), identity("recovered"), strings.NewReader("recovered")); err != nil {
		t.Fatal("failed copy did not release space/budget", err)
	}
	assertBytes(t, s, identity("recovered"), "recovered")
	t.Log("real bounded tmpfs write ENOSPC; no final object; same store next put/read passed")
}

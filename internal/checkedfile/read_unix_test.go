//go:build linux || darwin

package checkedfile

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// Preserve the original secret-loader metadata regressions at their new owner.
func TestOwnerAndStatChecks(t *testing.T) {
	const owner = 42
	dir := unix.Stat_t{Mode: unix.S_IFDIR | 0700, Uid: owner}
	if !trustedDirectory(dir, owner) {
		t.Fatal("private directory rejected")
	}
	dir.Uid = owner + 1
	if trustedDirectory(dir, owner) {
		t.Fatal("foreign owner accepted")
	}
	dir.Uid = owner
	dir.Mode = unix.S_IFDIR | unix.S_ISVTX | 0777
	if trustedDirectory(dir, owner) {
		t.Fatal("non-root sticky exception granted")
	}
	dir.Uid = 0
	if !trustedDirectory(dir, owner) {
		t.Fatal("root sticky ancestor rejected")
	}
	file := unix.Stat_t{Mode: unix.S_IFREG | 0600, Uid: owner, Nlink: 1, Size: 32}
	if !validFile(file, owner, 4096, true) {
		t.Fatal("private regular file rejected")
	}
	for _, change := range []string{"owner", "link", "mode", "size", "mtime", "ctime", "inode", "device"} {
		other := file
		switch change {
		case "owner":
			other.Uid++
		case "link":
			other.Nlink++
		case "mode":
			other.Mode |= 0040
		case "size":
			other.Size++
		case "mtime":
			other.Mtim.Sec++
		case "ctime":
			other.Ctim.Sec++
		case "inode":
			other.Ino++
		case "device":
			other.Dev++
		}
		if sameFile(file, other) {
			t.Fatal("changed descriptor state accepted")
		}
	}
	file.Uid++
	if validFile(file, owner, 4096, true) {
		t.Fatal("foreign file owner accepted")
	}
}

func TestPublicMaterialReadPolicy(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "synthetic.pem")
	if err := os.WriteFile(path, []byte("synthetic public material"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []os.FileMode{0400, 0600, 0444, 0644, 0640} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		body, err := ReadPublic(path, 64)
		if err != nil || string(body) != "synthetic public material" {
			t.Fatal("trusted public material rejected")
		}
		if mode != 0400 && mode != 0600 {
			if _, err := ReadSecret(path, 64); err != ErrUnavailable {
				t.Fatal("public material accepted as private key")
			}
		}
	}
	for _, mode := range []os.FileMode{0664, 0666, 0700, 0601, 0602} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadPublic(path, 64); err != ErrUnavailable {
			t.Fatal("writable/executable material accepted")
		}
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int64{0, 1, 1<<20 + 1} {
		if _, err := ReadPublic(path, limit); err != ErrUnavailable {
			t.Fatal("invalid size budget accepted")
		}
	}
}

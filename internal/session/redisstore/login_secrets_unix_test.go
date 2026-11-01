//go:build linux || darwin

package redisstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func secretDirectory(t *testing.T) string {
	t.Helper()
	// Tests canonicalize only their synthetic fixtures; the loader never follows
	// symlinks (including Darwin's /var or /tmp aliases).
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeSyntheticSecrets(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func rejectSecretPath(t *testing.T, path string) {
	t.Helper()
	s, err := LoadLoginSecrets(path)
	if s != nil || err != ErrLoginSecrets || errors.Unwrap(err) != nil || strings.Contains(err.Error(), path) {
		t.Fatal("unsafe path accepted or filesystem information exposed")
	}
}

func TestLoadLoginSecretsFilesystem(t *testing.T) {
	for _, mode := range []os.FileMode{0400, 0600} {
		dir := secretDirectory(t)
		path := filepath.Join(dir, "keys.json")
		writeSyntheticSecrets(t, path, syntheticSecrets())
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadLoginSecrets(path); err != nil {
			t.Fatal("private file rejected")
		}
	}
	for _, name := range []string{"symlink", "parent symlink", "hardlink", "directory", "fifo", "oversize", "empty", "public", "group", "executable", "writable parent", "missing", "relative", "unclean"} {
		t.Run(name, func(t *testing.T) {
			dir := secretDirectory(t)
			path := filepath.Join(dir, "keys.json")
			writeSyntheticSecrets(t, path, syntheticSecrets())
			switch name {
			case "symlink":
				link := filepath.Join(dir, "link")
				if err := os.Symlink(path, link); err != nil {
					t.Fatal(err)
				}
				path = link
			case "parent symlink":
				link := filepath.Join(dir, "link")
				if err := os.Symlink(dir, link); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(link, "keys.json")
			case "hardlink":
				if err := os.Link(path, filepath.Join(dir, "other")); err != nil {
					t.Fatal(err)
				}
			case "directory":
				path = dir
			case "fifo":
				path = filepath.Join(dir, "fifo")
				if err := unix.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				writeSyntheticSecrets(t, path, strings.Repeat("x", maxLoginSecretsBytes+1))
			case "empty":
				writeSyntheticSecrets(t, path, "")
			case "public":
				if err := os.Chmod(path, 0444); err != nil {
					t.Fatal(err)
				}
			case "group":
				if err := os.Chmod(path, 0640); err != nil {
					t.Fatal(err)
				}
			case "executable":
				if err := os.Chmod(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "writable parent":
				if err := os.Chmod(dir, 0777); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := os.Chmod(dir, 0700); err != nil {
						t.Error(err)
					}
				})
			case "missing":
				path += ".missing"
			case "relative":
				path = "keys.json"
			case "unclean":
				path = dir + "/./keys.json"
			}
			rejectSecretPath(t, path)
		})
	}
}

func TestLoadLoginSecretsAtomicReplacement(t *testing.T) {
	dir := secretDirectory(t)
	path := filepath.Join(dir, "keys.json")
	writeSyntheticSecrets(t, path, syntheticSecrets())
	old, err := LoadLoginSecrets(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(dir, "replacement.json")
	writeSyntheticSecrets(t, replacement, strings.ReplaceAll(syntheticSecrets(), "active", "next"))
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	next, err := LoadLoginSecrets(path)
	if err != nil || old.current != "active" || next.current != "next" || old.limiting != next.limiting {
		t.Fatal("replacement mutated an already loaded snapshot")
	}
}

func TestLoginSecretsOwnerAndStatChecks(t *testing.T) {
	const owner = 42
	dir := unix.Stat_t{Mode: unix.S_IFDIR | 0700, Uid: owner}
	if !trustedSecretsDirectory(dir, owner) {
		t.Fatal("private directory rejected")
	}
	dir.Uid = owner + 1
	if trustedSecretsDirectory(dir, owner) {
		t.Fatal("foreign owner accepted")
	}
	dir.Uid = owner
	dir.Mode = unix.S_IFDIR | unix.S_ISVTX | 0777
	if trustedSecretsDirectory(dir, owner) {
		t.Fatal("non-root sticky exception granted")
	}
	dir.Uid = 0
	if !trustedSecretsDirectory(dir, owner) {
		t.Fatal("root sticky ancestor rejected")
	}
	file := unix.Stat_t{Mode: unix.S_IFREG | 0600, Uid: owner, Nlink: 1, Size: 32}
	if !privateSecretsFile(file, owner) {
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
		if sameSecretsFile(file, other) {
			t.Fatal("changed descriptor state accepted")
		}
	}
	file.Uid++
	if privateSecretsFile(file, owner) {
		t.Fatal("foreign file owner accepted")
	}
}

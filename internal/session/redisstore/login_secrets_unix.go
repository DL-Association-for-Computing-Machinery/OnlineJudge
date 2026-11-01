//go:build linux || darwin

package redisstore

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func readLoginSecrets(path string) ([]byte, error) {
	if len(path) < 2 || len(path) > 4096 || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.IndexByte(path, 0) >= 0 {
		return nil, ErrLoginSecrets
	}
	parts := strings.Split(path[1:], "/")
	if len(parts) > 64 {
		return nil, ErrLoginSecrets
	}
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_DIRECTORY
	root, err := unix.Open("/", flags, 0)
	if err != nil {
		return nil, ErrLoginSecrets
	}
	fds := []int{root}
	defer func() {
		for i := len(fds) - 1; i >= 0; i-- {
			_ = unix.Close(fds[i])
		}
	}()
	owner := uint32(os.Geteuid())
	var st unix.Stat_t
	if unix.Fstat(root, &st) != nil || !trustedSecretsDirectory(st, owner) {
		return nil, ErrLoginSecrets
	}
	for _, part := range parts[:len(parts)-1] {
		fd, err := unix.Openat(fds[len(fds)-1], part, flags, 0)
		if err != nil {
			return nil, ErrLoginSecrets
		}
		fds = append(fds, fd)
		if unix.Fstat(fd, &st) != nil || !trustedSecretsDirectory(st, owner) {
			return nil, ErrLoginSecrets
		}
	}
	fd, err := unix.Openat(fds[len(fds)-1], parts[len(parts)-1], unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrLoginSecrets
	}
	f := os.NewFile(uintptr(fd), "login secrets")
	if f == nil {
		_ = unix.Close(fd)
		return nil, ErrLoginSecrets
	}
	var before, after unix.Stat_t
	if unix.Fstat(fd, &before) != nil || !privateSecretsFile(before, owner) {
		_ = f.Close()
		return nil, ErrLoginSecrets
	}
	body, readErr := io.ReadAll(io.LimitReader(f, maxLoginSecretsBytes+1))
	statErr := unix.Fstat(fd, &after)
	closeErr := f.Close()
	if readErr != nil || statErr != nil || closeErr != nil || len(body) > maxLoginSecretsBytes || int64(len(body)) != before.Size ||
		!privateSecretsFile(after, owner) || !sameSecretsFile(before, after) {
		clear(body)
		return nil, ErrLoginSecrets
	}
	return body, nil
}

func trustedSecretsDirectory(s unix.Stat_t, owner uint32) bool {
	if s.Mode&unix.S_IFMT != unix.S_IFDIR || (s.Uid != 0 && s.Uid != owner) {
		return false
	}
	// A root-owned sticky public ancestor protects entries owned by root/euid;
	// every next component is still separately opened and checked. No such
	// exception is granted to a caller-owned world-writable sticky directory.
	return s.Mode&0022 == 0 || (s.Uid == 0 && s.Mode&unix.S_ISVTX != 0)
}

func privateSecretsFile(s unix.Stat_t, owner uint32) bool {
	mode := s.Mode & 07777
	return s.Mode&unix.S_IFMT == unix.S_IFREG && (s.Uid == 0 || s.Uid == owner) && s.Nlink == 1 &&
		(mode == 0400 || mode == 0600) && s.Size > 0 && s.Size <= maxLoginSecretsBytes
}

func sameSecretsFile(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Uid == b.Uid && a.Gid == b.Gid && a.Mode == b.Mode && a.Nlink == b.Nlink &&
		a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim
}

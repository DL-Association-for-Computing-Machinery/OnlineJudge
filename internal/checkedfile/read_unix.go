//go:build linux || darwin

package checkedfile

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func read(path string, limit int64, private bool) ([]byte, error) {
	if limit < 1 || limit > 1<<20 || len(path) < 2 || len(path) > 4096 || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.IndexByte(path, 0) >= 0 {
		return nil, ErrUnavailable
	}
	parts := strings.Split(path[1:], "/")
	if len(parts) > 64 {
		return nil, ErrUnavailable
	}
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_DIRECTORY
	root, err := unix.Open("/", flags, 0)
	if err != nil {
		return nil, ErrUnavailable
	}
	fds := []int{root}
	defer func() {
		for i := len(fds) - 1; i >= 0; i-- {
			_ = unix.Close(fds[i])
		}
	}()
	owner := uint32(os.Geteuid())
	var st unix.Stat_t
	if unix.Fstat(root, &st) != nil || !trustedDirectory(st, owner) {
		return nil, ErrUnavailable
	}
	for _, part := range parts[:len(parts)-1] {
		fd, err := unix.Openat(fds[len(fds)-1], part, flags, 0)
		if err != nil {
			return nil, ErrUnavailable
		}
		fds = append(fds, fd)
		if unix.Fstat(fd, &st) != nil || !trustedDirectory(st, owner) {
			return nil, ErrUnavailable
		}
	}
	fd, err := unix.Openat(fds[len(fds)-1], parts[len(parts)-1], unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrUnavailable
	}
	f := os.NewFile(uintptr(fd), "configured file")
	if f == nil {
		_ = unix.Close(fd)
		return nil, ErrUnavailable
	}
	var before, after unix.Stat_t
	if unix.Fstat(fd, &before) != nil || !validFile(before, owner, limit, private) {
		_ = f.Close()
		return nil, ErrUnavailable
	}
	body, readErr := io.ReadAll(io.LimitReader(f, limit+1))
	statErr := unix.Fstat(fd, &after)
	closeErr := f.Close()
	if readErr != nil || statErr != nil || closeErr != nil || int64(len(body)) > limit || int64(len(body)) != before.Size || !validFile(after, owner, limit, private) || !sameFile(before, after) {
		clear(body)
		return nil, ErrUnavailable
	}
	return body, nil
}

func trustedDirectory(s unix.Stat_t, owner uint32) bool {
	if s.Mode&unix.S_IFMT != unix.S_IFDIR || (s.Uid != 0 && s.Uid != owner) {
		return false
	}
	return s.Mode&0022 == 0 || (s.Uid == 0 && s.Mode&unix.S_ISVTX != 0)
}

func validFile(s unix.Stat_t, owner uint32, limit int64, private bool) bool {
	mode := s.Mode & 07777
	allowed := mode == 0400 || mode == 0600
	if !private {
		allowed = mode&07133 == 0 && mode&0400 != 0
	}
	return s.Mode&unix.S_IFMT == unix.S_IFREG && (s.Uid == 0 || s.Uid == owner) && s.Nlink == 1 && allowed && s.Size > 0 && s.Size <= limit
}

func sameFile(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid && a.Nlink == b.Nlink && a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim
}

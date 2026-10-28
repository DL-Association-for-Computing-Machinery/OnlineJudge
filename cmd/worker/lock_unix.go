//go:build darwin || linux

package main

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"syscall"
)

type slotLock struct{ file *os.File }

// Resolve the account home, not HOME/TMPDIR or a caller-selected path: different
// checkouts and launch environments must contend for the same dedicated slot.
func lockM0() (*slotLock, error) {
	u, err := user.Current()
	if err != nil || !filepath.IsAbs(u.HomeDir) {
		return nil, errors.New("cannot resolve M0 owner home")
	}
	dir := filepath.Join(u.HomeDir, ".onlinejudge-m0")
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return nil, errors.New("cannot create M0 ownership directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return nil, errors.New("M0 ownership directory must be private and not a symlink")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return nil, errors.New("M0 ownership directory has wrong owner")
	}
	return acquireLock(filepath.Join(dir, "slot.lock"))
}

func acquireLock(path string) (*slotLock, error) {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, errors.New("cannot open M0 slot lock")
	}
	f := os.NewFile(uintptr(fd), path)
	fail := func(message string) (*slotLock, error) { f.Close(); return nil, errors.New(message) }
	var stat syscall.Stat_t
	if syscall.Fstat(fd, &stat) != nil || stat.Mode&syscall.S_IFMT != syscall.S_IFREG ||
		stat.Mode&0077 != 0 || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return fail("M0 lock must be an owner-private regular file with one link")
	}
	if syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return fail("M0 slot already owned or locking unavailable")
	}
	info, err := f.Stat()
	if err != nil || info.Size() != 0 {
		return fail("M0 slot quarantined; operator must confirm sandbox termination before recovery")
	}
	return &slotLock{file: f}, nil
}

func (l *slotLock) dirty() error {
	if _, err := l.file.WriteAt([]byte("active\n"), 0); err != nil {
		return errors.New("cannot persist M0 quarantine marker")
	}
	if err := l.file.Sync(); err != nil {
		return errors.New("cannot persist M0 quarantine marker")
	}
	return nil
}

func (l *slotLock) clean() error {
	if err := l.file.Truncate(0); err != nil {
		return errors.New("cannot clear M0 quarantine marker")
	}
	if err := l.file.Sync(); err != nil {
		return errors.New("cannot sync M0 quarantine marker")
	}
	return nil
}

// Close never removes the inode or clears an uncertain execution marker.
func (l *slotLock) Close() { l.file.Close() }

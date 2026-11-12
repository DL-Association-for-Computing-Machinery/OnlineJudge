//go:build darwin || linux

package main

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"syscall"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

type markerFile interface {
	WriteAt([]byte, int64) (int, error)
	Sync() error
	Truncate(int64) error
	Close() error
}

type slotLock struct {
	file   markerFile
	host   *os.File
	dual   bool
	marker string
}

// Resolve the account home, not HOME/TMPDIR or a caller-selected path: different
// checkouts and launch environments must contend for the same dedicated slot.
func lockM0(slot workerhost.M0Slot) (*slotLock, error) {
	return lockSlotMode(slot.LockName(), false)
}

func lockSlotMode(name string, recovery bool) (*slotLock, error) {
	return lockHostSlot(name, recovery, false)
}

func ownershipDirectory() (string, error) {
	u, err := user.Current()
	if err != nil || !filepath.IsAbs(u.HomeDir) {
		return "", errors.New("cannot resolve M0 owner home")
	}
	dir := filepath.Join(u.HomeDir, ".onlinejudge-m0")
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return "", errors.New("cannot create M0 ownership directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return "", errors.New("M0 ownership directory must be private and not a symlink")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return "", errors.New("M0 ownership directory has wrong owner")
	}
	return dir, nil
}

func acquireLock(path string) (*slotLock, error) {
	return acquireLockMode(path, false)
}

func acquireLockMode(path string, recovery bool) (*slotLock, error) {
	f, err := lockSlotFile(path, recovery)
	if err != nil {
		return nil, err
	}
	return &slotLock{file: f}, nil
}

func lockSlotFile(path string, recovery bool) (*os.File, error) {
	f, err := openOwnershipLock(path, true)
	if err != nil {
		return nil, err
	}
	if err := flockOwnership(f, syscall.LOCK_EX, recovery); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func openOwnershipLock(path string, create bool) (*os.File, error) {
	flags := syscall.O_RDWR | syscall.O_NOFOLLOW | syscall.O_CLOEXEC | syscall.O_NONBLOCK
	if create {
		flags |= syscall.O_CREAT
	}
	fd, err := syscall.Open(path, flags, 0600)
	if !create && errors.Is(err, syscall.ENOENT) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, errors.New("cannot open M0 ownership lock")
	}
	f := os.NewFile(uintptr(fd), path)
	var stat syscall.Stat_t
	if syscall.Fstat(fd, &stat) != nil || stat.Mode&syscall.S_IFMT != syscall.S_IFREG ||
		stat.Mode&0077 != 0 || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		_ = f.Close()
		return nil, errors.New("M0 lock must be an owner-private regular file with one link")
	}
	return f, nil
}

func flockOwnership(f *os.File, how int, allowDirty bool) error {
	if syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB) != nil {
		return errors.New("M0 slot already owned or locking unavailable")
	}
	info, err := f.Stat()
	if err != nil || (!allowDirty && info.Size() != 0) {
		return errors.New("M0 slot quarantined; operator must confirm sandbox termination before recovery")
	}
	return nil
}

func (l *slotLock) dirty() error {
	marker := l.marker
	if marker == "" {
		marker = activeMarker
		if l.dual {
			marker = dualMarker
		}
	}
	if n, err := l.file.WriteAt([]byte(marker), 0); err != nil || n != len(marker) {
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
		// Truncate may already be visible. Restore the fence when possible; if
		// storage also rejects this write, durable quarantine remains uncertain.
		return errors.Join(errors.New("cannot sync M0 quarantine marker"), l.dirty())
	}
	return nil
}

// Close releases the slot before the host lock; neither inode is removed.
func (l *slotLock) Close() {
	_ = l.file.Close() // Marker writes are synced and checked before cleanup.
	if l.host != nil {
		_ = l.host.Close()
	}
}

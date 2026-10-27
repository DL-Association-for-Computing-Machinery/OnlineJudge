// Package materials provides bounded, verified reads of task-authorized materials.
package materials

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"regexp"
	"sync"
	"syscall"
)

// MaxFileBytes is the M0 per-file budget, not a production capacity target.
const MaxFileBytes = 1 << 20

// ErrUnavailable covers absent, corrupt, oversized or inaccessible material.
var ErrUnavailable = errors.New("material unavailable")
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// File is a verified private snapshot. Close releases its descriptor and budget.
type File struct {
	io.ReadCloser
	SHA256 string
	Size   int64
}

// Reader opens a verified snapshot by content digest; the service owns authorization.
type Reader interface {
	Open(context.Context, string) (*File, error)
}

// Local confines reads to a dedicated directory and at most four active snapshots.
// It is supported on the project's Linux/macOS hosts, never inside contestant sandboxes.
type Local struct {
	root  *os.Root
	slots chan struct{}
}

// OpenLocal pins the existing controller directory; startup never creates materials.
func OpenLocal(path string) (*Local, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrUnavailable
	}
	return &Local{root: root, slots: make(chan struct{}, 4)}, nil
}

// Close releases the directory descriptor after the server has drained requests.
func (s *Local) Close() error { return s.root.Close() }

// Open verifies bytes before releasing any response and never serves a mutable
// original after verification. Snapshots are unlinked immediately on these Unix hosts.
func (s *Local) Open(ctx context.Context, hash string) (*File, error) {
	if !digestPattern.MatchString(hash) {
		return nil, ErrUnavailable
	}
	select {
	case s.slots <- struct{}{}:
	default:
		return nil, ErrUnavailable
	}
	release := func() { <-s.slots }
	success := false
	defer func() {
		if !success {
			release()
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// NOFOLLOW rejects symlinks; NONBLOCK prevents an injected FIFO from hanging
	// before Stat can reject non-regular files. Root also confines path resolution.
	source, err := s.root.OpenFile(hash, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxFileBytes {
		return nil, ErrUnavailable
	}
	file, err := os.CreateTemp("", "oj-material-*")
	if err != nil {
		return nil, ErrUnavailable
	}
	defer func() {
		if !success {
			file.Close()
			_ = os.Remove(file.Name())
		}
	}()
	if err := os.Remove(file.Name()); err != nil {
		return nil, ErrUnavailable
	}
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, h), io.LimitReader(contextReader{ctx, source}, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if size > MaxFileBytes || hex.EncodeToString(h.Sum(nil)) != hash {
		return nil, ErrUnavailable
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	success = true
	return &File{ReadCloser: &snapshot{File: file, release: release}, SHA256: hash, Size: size}, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

type snapshot struct {
	*os.File
	release func()
	once    sync.Once
	err     error
}

func (s *snapshot) Close() error {
	s.once.Do(func() { s.err = s.File.Close(); s.release() })
	return s.err
}

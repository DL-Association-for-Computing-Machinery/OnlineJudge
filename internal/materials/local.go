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

// Verify reads the named file and checks its digest without creating a snapshot.
func (s *Local) Verify(ctx context.Context, hash string) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer func() { <-s.slots }()
	source, err := s.source(hash)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	_, err = verify(ctx, source, io.Discard, hash)
	return err
}

// Open verifies bytes before releasing any response and never serves a mutable
// original after verification. Snapshots are unlinked immediately on these Unix hosts.
func (s *Local) Open(ctx context.Context, hash string) (*File, error) {
	if err := s.acquire(ctx); err != nil {
		return nil, err
	}
	release := func() { <-s.slots }
	success := false
	defer func() {
		if !success {
			release()
		}
	}()
	source, err := s.source(hash)
	if err != nil {
		return nil, ErrUnavailable
	}
	// Cleanup does not change the completed operation or its primary error.
	defer func() { _ = source.Close() }()
	file, err := os.CreateTemp("", "oj-material-*")
	if err != nil {
		return nil, ErrUnavailable
	}
	defer func() {
		if !success {
			_ = file.Close() // Cleanup; the operation outcome is checked separately.
			_ = os.Remove(file.Name())
		}
	}()
	if err := os.Remove(file.Name()); err != nil {
		return nil, ErrUnavailable
	}
	size, err := verify(ctx, source, file, hash)
	if err != nil {
		return nil, err
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

func (s *Local) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case s.slots <- struct{}{}:
		return nil
	default:
		return ErrUnavailable
	}
}

func (s *Local) source(hash string) (*os.File, error) {
	if !digestPattern.MatchString(hash) {
		return nil, ErrUnavailable
	}
	// NOFOLLOW rejects symlinks; NONBLOCK prevents a FIFO from hanging before Stat.
	file, err := s.root.OpenFile(hash, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrUnavailable
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxFileBytes {
		_ = file.Close()
		return nil, ErrUnavailable
	}
	return file, nil
}

func verify(ctx context.Context, source io.Reader, snapshot io.Writer, hash string) (int64, error) {
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(snapshot, h), io.LimitReader(contextReader{ctx, source}, MaxFileBytes+1))
	if err != nil {
		return 0, err
	}
	if size > MaxFileBytes || hex.EncodeToString(h.Sum(nil)) != hash {
		return 0, ErrUnavailable
	}
	return size, ctx.Err()
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

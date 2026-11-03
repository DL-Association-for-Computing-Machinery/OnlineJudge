// Package materialstore stores private immutable objects before business references
// are committed. Authorization belongs to the calling business service.
package materialstore

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
)

var (
	ErrInvalid     = errors.New("invalid material request or configuration")
	ErrUnavailable = errors.New("material storage unavailable")
	ErrNotFound    = errors.New("material not found")
	ErrIntegrity   = errors.New("material integrity failure")
	ErrBusy        = errors.New("material operation budget exhausted")
	ErrClosed      = errors.New("material storage closed")
	// ErrCommitUnknown means an object may already be published. Retry the same
	// object; never publish database references based only on a failed call.
	ErrCommitUnknown = errors.New("material persistence outcome unknown")
)

// Object is a logical content identity, never a caller-selected filesystem path.
type Object struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Store separates immutable storage from problem formats and access control.
type Store interface {
	Put(context.Context, Object, io.Reader) (Object, error)
	Open(context.Context, Object) (*Snapshot, error)
}

// Config contains explicit per-instance budgets, not a shared disk quota.
type Config struct {
	Root           string
	MaxObjectBytes int64
	MaxInFlight    int
}

// Local pins a pre-existing private directory on a trusted Linux filesystem.
// Callers must not mutate published objects or replace its trusted ancestors.
type Local struct {
	root      *os.Root
	directory *os.File
	maximum   int64
	limit     int
	mu        sync.Mutex
	active    int
	closed    bool
	closeErr  error
	ops       operations
}

type operations struct {
	syncFile      func(*os.File) error
	syncDirectory func(*os.File) error
	link          func(string, string) error
}

// OpenLocal checks an existing root; it never creates, repairs or cleans it.
// The root must already be durably provisioned by trusted maintenance. Linux
// file/directory Sync semantics are required; no fallback ignores Sync errors.
func OpenLocal(config Config) (*Local, error) {
	if runtime.GOOS != "linux" || !filepath.IsAbs(config.Root) || filepath.Clean(config.Root) != config.Root ||
		config.MaxObjectBytes < 1 || config.MaxObjectBytes > 1<<40 || config.MaxInFlight < 1 || config.MaxInFlight > 64 {
		return nil, ErrInvalid
	}
	before, err := os.Lstat(config.Root)
	if err != nil || !private(before, true) {
		return nil, errors.Join(ErrUnavailable, err)
	}
	root, err := os.OpenRoot(config.Root)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	dir, err := root.OpenFile(".", os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		_ = root.Close()
		return nil, errors.Join(ErrUnavailable, err)
	}
	after, err := dir.Stat()
	if err != nil || !private(after, true) || !os.SameFile(before, after) {
		_ = dir.Close()
		_ = root.Close()
		return nil, errors.Join(ErrUnavailable, err)
	}
	s := &Local{root: root, directory: dir, maximum: config.MaxObjectBytes, limit: config.MaxInFlight}
	s.ops = operations{syncFile: (*os.File).Sync, syncDirectory: (*os.File).Sync, link: root.Link}
	return s, nil
}

func private(info os.FileInfo, directory bool) bool {
	if info == nil {
		return false
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Geteuid()) {
		return false
	}
	if directory {
		return info.Mode() == os.ModeDir|0700
	}
	return info.Mode() == 0600
}

func (s *Local) valid(object Object) bool {
	if object.Size < 0 || object.Size > s.maximum || len(object.SHA256) != 64 {
		return false
	}
	for _, c := range object.SHA256 {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func (s *Local) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if s.active >= s.limit {
		return ErrBusy
	}
	s.active++
	return nil
}

func (s *Local) release() { s.mu.Lock(); s.active--; s.mu.Unlock() }

// Close refuses to close an active store. Drain operations and close every
// snapshot before retrying; repeated successful Close is idempotent.
func (s *Local) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	if s.active != 0 {
		return ErrBusy
	}
	s.closed = true
	s.closeErr = errors.Join(s.directory.Close(), s.root.Close())
	return s.closeErr
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

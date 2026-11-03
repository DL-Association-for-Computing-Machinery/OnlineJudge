package materialstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"syscall"
)

func (s *Local) temporary() (*os.File, string, error) {
	name := ".pending-" + rand.Text()
	f, err := s.root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, "", errors.Join(ErrUnavailable, err)
	}
	// umask may remove owner permissions from a newly created file. Set the
	// documented mode through our own descriptor before accepting any bytes.
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		_ = s.root.Remove(name)
		return nil, "", errors.Join(ErrUnavailable, err)
	}
	return f, name, nil
}

// Put validates the actual stream, syncs it, then atomically links a new object
// without replacement and syncs the directory. Existing bytes are revalidated
// and synced too. The reader must be trusted and bounded in blocking behavior;
// context cannot interrupt an arbitrary Read, Write or filesystem Sync call.
func (s *Local) Put(ctx context.Context, object Object, reader io.Reader) (Object, error) {
	if !s.valid(object) || reader == nil {
		return Object{}, ErrInvalid
	}
	if err := s.acquire(ctx); err != nil {
		return Object{}, err
	}
	defer s.release()
	f, name, err := s.temporary()
	if err != nil {
		return Object{}, err
	}
	defer func() { _ = f.Close(); _ = s.root.Remove(name) }()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(contextReader{ctx, reader}, object.Size+1))
	if err != nil {
		return Object{}, errors.Join(ErrUnavailable, err)
	}
	if n != object.Size || hex.EncodeToString(hash.Sum(nil)) != object.SHA256 {
		return Object{}, ErrIntegrity
	}
	if err := ctx.Err(); err != nil {
		return Object{}, err
	}
	if err := s.ops.syncFile(f); err != nil {
		return Object{}, errors.Join(ErrUnavailable, err)
	}
	if err := f.Close(); err != nil {
		return Object{}, errors.Join(ErrUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return Object{}, err
	}
	if err := s.ops.link(name, object.SHA256); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return object, errors.Join(ErrCommitUnknown, err)
		}
		// Another process may have published the name before syncing its own
		// directory. Visibility alone is not a durable acknowledgment.
		if err := s.confirm(ctx, object); err != nil {
			return object, err
		}
	}
	// After link, cancellation never skips persistence/cleanup. A later error
	// leaves a possibly committed object; do not unlink the published name.
	if err := s.root.Remove(name); err != nil {
		return object, errors.Join(ErrCommitUnknown, err)
	}
	if err := s.ops.syncDirectory(s.directory); err != nil {
		return object, errors.Join(ErrCommitUnknown, err)
	}
	if err := ctx.Err(); err != nil {
		return object, errors.Join(ErrCommitUnknown, err)
	}
	return object, nil
}

func (s *Local) source(object Object) (*os.File, error) {
	f, err := s.root.OpenFile(object.SHA256, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	info, err := f.Stat()
	if err != nil || !private(info, false) || info.Size() != object.Size {
		_ = f.Close()
		return nil, errors.Join(ErrIntegrity, err)
	}
	return f, nil
}

func (s *Local) confirm(ctx context.Context, object Object) error {
	f, err := s.source(object)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(contextReader{ctx, f}, object.Size+1))
	if err != nil {
		return errors.Join(ErrCommitUnknown, err)
	}
	if n != object.Size || hex.EncodeToString(hash.Sum(nil)) != object.SHA256 {
		return ErrIntegrity
	}
	if err := s.ops.syncFile(f); err != nil {
		return errors.Join(ErrCommitUnknown, err)
	}
	if err := f.Close(); err != nil {
		return errors.Join(ErrCommitUnknown, err)
	}
	return nil
}

package materialstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sync"
)

// Snapshot contains only fully verified bytes. Close releases the private
// anonymous disk file and the operation slot; it must be called by the caller.
type Snapshot struct {
	Object  Object
	file    *os.File
	ctx     context.Context
	release func()
	once    sync.Once
	err     error
}

// Read obeys the Open context and never reads the mutable stored original.
func (s *Snapshot) Read(p []byte) (int, error) { return (contextReader{s.ctx, s.file}).Read(p) }

// Close is idempotent, including when cleanup reports an error.
func (s *Snapshot) Close() error {
	s.once.Do(func() { s.err = s.file.Close(); s.release() })
	return s.err
}

// Verify checks a private object's size and digest without writing a snapshot.
func (s *Local) Verify(ctx context.Context, object Object) error {
	if !s.valid(object) {
		return ErrInvalid
	}
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	source, err := s.source(object)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	return verifyObject(ctx, object, source, io.Discard)
}

// Open copies and verifies the entire object before returning any bytes. It
// establishes read integrity, not a durable acknowledgment of a prior failed Put.
func (s *Local) Open(ctx context.Context, object Object) (*Snapshot, error) {
	if !s.valid(object) {
		return nil, ErrInvalid
	}
	if err := s.acquire(ctx); err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			s.release()
		}
	}()
	source, err := s.source(object)
	if err != nil {
		return nil, err
	}
	defer func() { _ = source.Close() }()
	file, name, err := s.temporary()
	if err != nil {
		return nil, err
	}
	defer func() {
		if !success {
			_ = file.Close()
			_ = s.root.Remove(name)
		}
	}()
	if err := s.root.Remove(name); err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	if err := verifyObject(ctx, object, source, file); err != nil {
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	success = true
	return &Snapshot{Object: object, file: file, ctx: ctx, release: s.release}, nil
}

func verifyObject(ctx context.Context, object Object, source io.Reader, snapshot io.Writer) error {
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(snapshot, hash), io.LimitReader(contextReader{ctx, source}, object.Size+1))
	if err != nil {
		return errors.Join(ErrUnavailable, err)
	}
	if n != object.Size || hex.EncodeToString(hash.Sum(nil)) != object.SHA256 {
		return ErrIntegrity
	}
	return ctx.Err()
}

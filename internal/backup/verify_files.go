package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"syscall"
)

func pinDirectory(before os.FileInfo, open func() (*os.Root, error)) (*os.Root, error) {
	if !privateDirectory(before) {
		return nil, errors.New("backup directory must be private and must not be a symlink")
	}
	dir, err := open()
	if err != nil {
		return nil, errors.New("backup directory unavailable")
	}
	after, err := dir.Stat(".")
	if err != nil || !privateDirectory(after) || !os.SameFile(before, after) {
		_ = dir.Close()
		return nil, errors.New("backup directory changed while opening")
	}
	return dir, nil
}

func childDirectory(root *os.Root, name string) (*os.Root, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, errors.New("backup material directory unavailable")
	}
	return pinDirectory(info, func() (*os.Root, error) { return root.OpenRoot(name) })
}

func privateDirectory(info os.FileInfo) bool {
	return info.Mode() == os.ModeDir|0700 || info.Mode() == os.ModeDir|0500
}

func privateFile(info os.FileInfo) bool { return info.Mode() == 0600 || info.Mode() == 0400 }

func openPackageFile(root *os.Root, name string) (*os.File, os.FileInfo, error) {
	before, err := root.Lstat(name)
	if err != nil || !privateFile(before) {
		return nil, nil, errors.New("backup entry must be a private regular file")
	}
	// Lstat rejects common malformed inputs before opening them; flags also
	// prevent following a replacement symlink or blocking on a replaced FIFO.
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, errors.New("backup file unavailable")
	}
	after, err := file.Stat()
	if err != nil || !privateFile(after) || !os.SameFile(before, after) {
		_ = file.Close()
		return nil, nil, errors.New("backup file changed while opening")
	}
	return file, after, nil
}

func checkFile(ctx context.Context, root *os.Root, name string, expected FileIdentity, consume func(io.Reader) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, info, err := openPackageFile(root, name)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if info.Size() != expected.Bytes {
		return errors.New("backup file length did not match")
	}
	h := sha256.New()
	source := cancelReader{ctx, file}
	limited := &io.LimitedReader{R: source, N: expected.Bytes}
	if err := consume(io.TeeReader(limited, h)); err != nil {
		return err
	}
	var extra [1]byte
	if _, err := io.ReadFull(source, extra[:]); limited.N != 0 || err != io.EOF || hex.EncodeToString(h.Sum(nil)) != expected.SHA256 {
		return errors.New("backup file bytes did not match")
	}
	return nil
}

func verifyRootNames(ctx context.Context, root *os.Root) error {
	if err := checkNames(ctx, root, 5, func(name string) bool {
		switch name {
		case archiveName, inventoryName, manifestName, judgeDir, privateDir:
			return true
		default:
			return false
		}
	}); err != nil {
		return errors.New("backup contains unexpected or missing top-level entries")
	}
	return nil
}

func checkNames(ctx context.Context, root *os.Root, want int64, allowed func(string) bool) error {
	file, err := root.OpenFile(".", os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		names, err := file.Readdirnames(128)
		for _, name := range names {
			total++
			if total > want || !allowed(name) {
				return errors.New("unexpected backup directory entry")
			}
		}
		if err == io.EOF {
			if total != want {
				return errors.New("missing backup directory entry")
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
}

type cancelReader struct {
	ctx context.Context
	r   io.Reader
}

func (r cancelReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

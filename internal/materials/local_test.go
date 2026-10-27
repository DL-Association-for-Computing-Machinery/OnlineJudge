package materials

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestLocalVerifiedSnapshot(t *testing.T) {
	dir := t.TempDir()
	data := "private input\n"
	hash := checksum([]byte(data))
	path := filepath.Join(dir, hash)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	file, err := reader.Open(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	// Replacing the original after verification cannot change the response.
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(file)
	if err != nil || string(got) != data || file.Size != int64(len(data)) || file.SHA256 != hash {
		t.Fatalf("snapshot = %q, %v", got, err)
	}
	if _, err := reader.Open(context.Background(), hash); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("corrupt read = %v", err)
	}
}

func TestLocalRejectsUnsafeEntries(t *testing.T) {
	for _, kind := range []string{"missing", "symlink", "fifo", "directory", "oversized", "corrupt", "traversal"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			hash := checksum([]byte("expected"))
			path := filepath.Join(dir, hash)
			var err error
			switch kind {
			case "symlink":
				target := filepath.Join(t.TempDir(), "secret")
				if err := os.WriteFile(target, []byte("expected"), 0600); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(target, path)
			case "fifo":
				err = syscall.Mkfifo(path, 0600)
			case "directory":
				err = os.Mkdir(path, 0700)
			case "oversized":
				err = os.WriteFile(path, []byte(strings.Repeat("x", MaxFileBytes+1)), 0600)
			case "corrupt":
				err = os.WriteFile(path, []byte("wrong"), 0600)
			case "traversal":
				hash = "../secret"
			}
			if err != nil {
				t.Fatal(err)
			}
			reader, err := OpenLocal(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			// Repeated failures must release capacity too.
			for range 6 {
				if _, err := reader.Open(context.Background(), hash); !errors.Is(err, ErrUnavailable) {
					t.Fatalf("unsafe read = %v", err)
				}
			}
			if len(reader.slots) != 0 {
				t.Fatal("failed read leaked capacity")
			}
		})
	}
}

func TestLocalBudgetAndCancellation(t *testing.T) {
	dir := t.TempDir()
	hash := checksum(nil)
	if err := os.WriteFile(filepath.Join(dir, hash), nil, 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.Open(ctx, hash); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var files []*File
	for range 4 {
		f, err := reader.Open(context.Background(), hash)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
		defer f.Close()
	}
	if _, err := reader.Open(context.Background(), hash); !errors.Is(err, ErrUnavailable) {
		t.Fatal("unbounded snapshots")
	}
	files[0].Close()
	files[0].Close()
	f, err := reader.Open(context.Background(), hash)
	if err != nil {
		t.Fatal("close did not release capacity", err)
	}
	f.Close()
}

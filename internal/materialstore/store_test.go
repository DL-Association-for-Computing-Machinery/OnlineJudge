package materialstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func identity(data string) Object {
	h := sha256.Sum256([]byte(data))
	return Object{SHA256: hex.EncodeToString(h[:]), Size: int64(len(data))}
}

func privateRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// Go's numbered TempDir children inherit umask from a 0777 creation.
	// Provision the same explicit mode required of a real maintenance root.
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func local(t *testing.T, root string, limit int) *Local {
	t.Helper()
	s, err := OpenLocal(Config{Root: root, MaxObjectBytes: 128, MaxInFlight: limit})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func assertBytes(t *testing.T, s *Local, object Object, want string) {
	t.Helper()
	f, err := s.Open(context.Background(), object)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	if err != nil || string(data) != want || f.Object != object {
		t.Fatalf("snapshot mismatch: size=%d error=%v", len(data), err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPutReopenAndVerifiedSnapshot(t *testing.T) {
	root := privateRoot(t)
	s := local(t, root, 2)
	for _, data := range []string{"", strings.Repeat("a", 128), "private answer\n"} {
		object := identity(data)
		got, err := s.Put(context.Background(), object, strings.NewReader(data))
		if err != nil || got != object {
			t.Fatal("put", err)
		}
		assertBytes(t, s, object, data)
		info, err := os.Stat(filepath.Join(root, object.SHA256))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("private object mode", err)
		}
	}
	object := identity("private answer\n")
	f, err := s.Open(context.Background(), object)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, object.SHA256), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(f)
	if err != nil || string(got) != "private answer\n" {
		t.Fatal("snapshot changed after original modification")
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(context.Background(), object); !errors.Is(err, ErrIntegrity) {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := local(t, root, 1)
	assertBytes(t, reopened, identity(""), "")
}

func TestInputBudgetsFailuresAndCancellation(t *testing.T) {
	s := local(t, privateRoot(t), 1)
	for _, tc := range []struct {
		name   string
		object Object
		source string
		want   error
	}{
		{"long", identity("a"), "ab", ErrIntegrity},
		{"short", identity("ab"), "a", ErrIntegrity},
		{"wrong hash", identity("a"), "b", ErrIntegrity},
		{"over budget", identity(strings.Repeat("a", 129)), "", ErrInvalid},
		{"negative", Object{SHA256: identity("").SHA256, Size: -1}, "", ErrInvalid},
		{"path", Object{SHA256: "../private", Size: 0}, "", ErrInvalid},
		{"uppercase", Object{SHA256: strings.ToUpper(identity("").SHA256), Size: 0}, "", ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.Put(context.Background(), tc.object, strings.NewReader(tc.source)); !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
		})
	}
	if _, err := s.Put(context.Background(), identity(""), nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Put(ctx, identity(""), strings.NewReader("")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := s.Open(ctx, identity("")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(), identity("abc"), brokenReader{}); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	if _, err := s.Put(ctx, identity("abc"), cancelReader{cancel}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(), identity("ok"), strings.NewReader("ok")); err != nil {
		t.Fatal("failure leaked budget", err)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type cancelReader struct{ cancel context.CancelFunc }

func (r cancelReader) Read(p []byte) (int, error) { r.cancel(); copy(p, "a"); return 1, nil }

func TestSnapshotBudgetAndConcurrentClose(t *testing.T) {
	s := local(t, privateRoot(t), 1)
	object := identity("abc")
	if _, err := s.Put(context.Background(), object, strings.NewReader("abc")); err != nil {
		t.Fatal(err)
	}
	f, err := s.Open(context.Background(), object)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(context.Background(), object); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(), object, strings.NewReader("abc")); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	if err := s.Close(); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := f.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(context.Background(), object); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(), object, strings.NewReader("abc")); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestUnsafeRootsAndObjects(t *testing.T) {
	for _, kind := range []string{"symlink", "fifo", "directory", "mode", "corrupt", "missing"} {
		t.Run(kind, func(t *testing.T) {
			root := privateRoot(t)
			object := identity("expected")
			path := filepath.Join(root, object.SHA256)
			s := local(t, root, 1)
			var err error
			switch kind {
			case "symlink":
				target := filepath.Join(t.TempDir(), "other")
				err = os.WriteFile(target, []byte("expected"), 0600)
				if err == nil {
					err = os.Symlink(target, path)
				}
			case "fifo":
				err = syscall.Mkfifo(path, 0600)
			case "directory":
				err = os.Mkdir(path, 0700)
			case "mode":
				err = os.WriteFile(path, []byte("expected"), 0644)
			case "corrupt":
				err = os.WriteFile(path, []byte("replaced"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Open(context.Background(), object); err == nil {
				t.Fatal("unsafe read accepted")
			}
			if kind != "missing" {
				before, _ := os.Lstat(path)
				if _, err := s.Put(context.Background(), object, strings.NewReader("expected")); err == nil {
					t.Fatal("unsafe existing object accepted")
				}
				after, _ := os.Lstat(path)
				if !os.SameFile(before, after) {
					t.Fatal("existing object overwritten")
				}
			}
		})
	}
	root := privateRoot(t)
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	for _, config := range []Config{{Root: link, MaxObjectBytes: 1, MaxInFlight: 1}, {Root: root, MaxObjectBytes: 0, MaxInFlight: 1}, {Root: root, MaxObjectBytes: 1, MaxInFlight: 0}, {Root: "relative", MaxObjectBytes: 1, MaxInFlight: 1}} {
		if s, err := OpenLocal(config); err == nil {
			_ = s.Close()
			t.Fatal("unsafe configuration accepted")
		}
	}
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if s, err := OpenLocal(Config{Root: root, MaxObjectBytes: 1, MaxInFlight: 1}); err == nil {
		_ = s.Close()
		t.Fatal("nonprivate root accepted")
	}
}

func TestTwoInstancesPublishOneImmutableObject(t *testing.T) {
	root := privateRoot(t)
	a := local(t, root, 16)
	b := local(t, root, 16)
	data := strings.Repeat("x", 128)
	object := identity(data)
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Go(func() {
			s := a
			if i%2 == 1 {
				s = b
			}
			if _, err := s.Put(context.Background(), object, bytes.NewBufferString(data)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	assertBytes(t, a, object, data)
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal("unexpected published or temporary objects", err)
	}
	before, err := os.Stat(filepath.Join(root, object.SHA256))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Put(context.Background(), object, strings.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(filepath.Join(root, object.SHA256))
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("idempotent put replaced inode", err)
	}
}

func TestSnapshotCancellationAndReadFailuresReleaseCapacity(t *testing.T) {
	s := local(t, privateRoot(t), 1)
	object := identity("payload")
	if _, err := s.Put(context.Background(), object, strings.NewReader("payload")); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if _, err := s.Open(context.Background(), identity("missing")); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	f, err := s.Open(ctx, object)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := f.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	assertBytes(t, s, object, "payload")
}

func TestForeignOwnerRootAndObjects(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("ownership fixtures require an explicit isolated root container")
	}
	root := privateRoot(t)
	s := local(t, root, 1)
	object := identity("payload")
	path := filepath.Join(root, object.SHA256)
	if err := os.WriteFile(path, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 65532, 65532); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(context.Background(), object); !errors.Is(err, ErrIntegrity) {
		t.Fatal("foreign object read", err)
	}
	if _, err := s.Put(context.Background(), object, strings.NewReader("payload")); !errors.Is(err, ErrIntegrity) {
		t.Fatal("foreign object replaced", err)
	}
	foreign := privateRoot(t)
	if err := os.Chown(foreign, 65532, 65532); err != nil {
		t.Fatal(err)
	}
	if got, err := OpenLocal(Config{Root: foreign, MaxObjectBytes: 128, MaxInFlight: 1}); err == nil {
		_ = got.Close()
		t.Fatal("foreign root accepted")
	}
}

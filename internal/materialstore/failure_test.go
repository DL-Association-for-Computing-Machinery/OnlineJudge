package materialstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestPersistenceFailuresAndOriginalObjectRetry(t *testing.T) {
	for _, phase := range []string{"file-sync", "link-ack", "directory-sync", "existing-file-sync", "existing-directory-sync", "cancel-after-link", "existing-cancel"} {
		t.Run(phase, func(t *testing.T) {
			root := privateRoot(t)
			s := local(t, root, 1)
			object := identity("payload")
			if strings.HasPrefix(phase, "existing-") {
				if _, err := s.Put(context.Background(), object, strings.NewReader("payload")); err != nil {
					t.Fatal(err)
				}
			}
			original := s.ops
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			directorySynced := false
			s.ops.syncDirectory = func(f *os.File) error {
				directorySynced = true
				if phase == "directory-sync" || phase == "existing-directory-sync" {
					return syscall.ENOSPC
				}
				return original.syncDirectory(f)
			}
			s.ops.syncFile = func(f *os.File) error {
				if phase == "file-sync" || (phase == "existing-file-sync" && filepath.Base(f.Name()) == object.SHA256) {
					return syscall.EIO
				}
				if phase == "existing-cancel" && filepath.Base(f.Name()) == object.SHA256 {
					cancel()
				}
				return original.syncFile(f)
			}
			s.ops.link = func(from, to string) error {
				err := original.link(from, to)
				if err == nil && phase == "link-ack" {
					return syscall.EIO
				}
				if err == nil && phase == "cancel-after-link" {
					cancel()
				}
				return err
			}
			_, err := s.Put(ctx, object, strings.NewReader("payload"))
			if phase == "file-sync" {
				if !errors.Is(err, ErrUnavailable) {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(root, object.SHA256)); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("published before successful file sync")
				}
			} else if !errors.Is(err, ErrCommitUnknown) {
				t.Fatal("possibly published outcome was not unknown", err)
			}
			if (phase == "cancel-after-link" || phase == "existing-cancel") && (!directorySynced || !errors.Is(err, context.Canceled)) {
				t.Fatal("cancel skipped persistence cleanup", err)
			}
			s.ops = original
			if _, err := s.Put(context.Background(), object, strings.NewReader("payload")); err != nil {
				t.Fatal("same object retry", err)
			}
			assertBytes(t, s, object, "payload")
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 1 {
				t.Fatal("temporary write leaked", err)
			}
		})
	}
}

func TestExistingObjectStillValidatesIncomingBytesAndSyncs(t *testing.T) {
	s := local(t, privateRoot(t), 1)
	object := identity("correct")
	if _, err := s.Put(context.Background(), object, strings.NewReader("correct")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(), object, strings.NewReader("changed")); !errors.Is(err, ErrIntegrity) {
		t.Fatal("existing hash skipped source verification", err)
	}
	original := s.ops
	var events []string
	s.ops.syncFile = func(f *os.File) error { events = append(events, "file"); return original.syncFile(f) }
	s.ops.syncDirectory = func(f *os.File) error { events = append(events, "directory"); return original.syncDirectory(f) }
	s.ops.link = func(a, b string) error { events = append(events, "link"); return original.link(a, b) }
	if _, err := s.Put(context.Background(), object, strings.NewReader("correct")); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "file,link,file,directory" {
		t.Fatal("existing object was not independently synced", events)
	}
}

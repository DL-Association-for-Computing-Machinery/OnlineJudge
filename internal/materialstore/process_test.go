package materialstore

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestProcessHelper(t *testing.T) {
	mode := os.Getenv("OJ_MATERIALSTORE_CHILD_MODE")
	if mode == "" {
		return
	}
	if mode == "restrictive" {
		syscall.Umask(0777)
	}
	s, err := OpenLocal(Config{Root: os.Getenv("OJ_MATERIALSTORE_CHILD_ROOT"), MaxObjectBytes: 128, MaxInFlight: 1})
	if err != nil {
		os.Exit(91)
	}
	original := s.ops
	if mode == "written" {
		s.ops.syncFile = func(*os.File) error { os.Exit(99); return nil }
	}
	if mode == "file-synced" {
		s.ops.syncFile = func(f *os.File) error {
			if err := original.syncFile(f); err != nil {
				return err
			}
			os.Exit(99)
			return nil
		}
	}
	if mode == "linked" {
		s.ops.link = func(a, b string) error {
			if err := original.link(a, b); err != nil {
				return err
			}
			os.Exit(99)
			return nil
		}
	}
	if mode == "directory-synced" {
		s.ops.syncDirectory = func(f *os.File) error {
			if err := original.syncDirectory(f); err != nil {
				return err
			}
			os.Exit(99)
			return nil
		}
	}
	if mode == "concurrent" {
		if _, err := io.WriteString(os.Stdout, "ready\n"); err != nil {
			os.Exit(92)
		}
		if _, err := io.ReadFull(os.Stdin, make([]byte, 1)); err != nil {
			os.Exit(93)
		}
	}
	var reader io.Reader = strings.NewReader("payload")
	if mode == "partial" {
		reader = &crashReader{}
	}
	if _, err := s.Put(context.Background(), identity("payload"), reader); err != nil {
		os.Exit(94)
	}
	if err := s.Close(); err != nil {
		os.Exit(95)
	}
}

type crashReader struct{ read bool }

func (r *crashReader) Read(p []byte) (int, error) {
	if r.read {
		os.Exit(99)
	}
	r.read = true
	copy(p, "pay")
	return 3, nil
}

func child(t *testing.T, root, mode string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProcessHelper$")
	cmd.Env = append(os.Environ(), "OJ_MATERIALSTORE_CHILD_MODE="+mode, "OJ_MATERIALSTORE_CHILD_ROOT="+root)
	return cmd
}

func TestAbruptProcessExitNeverPublishesPartialObjects(t *testing.T) {
	for _, phase := range []string{"partial", "written", "file-synced", "linked", "directory-synced"} {
		t.Run(phase, func(t *testing.T) {
			root := privateRoot(t)
			cmd := child(t, root, phase)
			err := cmd.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 99 {
				t.Fatal("crash helper did not reach intended boundary", err)
			}
			s := local(t, root, 1)
			object := identity("payload")
			if phase == "linked" || phase == "directory-synced" {
				assertBytes(t, s, object, "payload")
			} else {
				if _, err := s.Open(context.Background(), object); !errors.Is(err, ErrNotFound) {
					t.Fatal("partial object visible", err)
				}
			}
			if _, err := s.Put(context.Background(), object, strings.NewReader("payload")); err != nil {
				t.Fatal(err)
			}
			assertBytes(t, s, object, "payload")
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.Name() == object.SHA256 {
					continue
				}
				if !strings.HasPrefix(entry.Name(), ".pending-") {
					t.Fatal("unexpected crash artifact")
				}
				if _, err := s.Open(context.Background(), Object{SHA256: entry.Name(), Size: 3}); !errors.Is(err, ErrInvalid) {
					t.Fatal("staging name accepted as object", err)
				}
			}
		})
	}
}

func TestIndependentProcessesPublishSameObject(t *testing.T) {
	root := privateRoot(t)
	var commands []*exec.Cmd
	var inputs []io.WriteCloser
	for range 2 {
		cmd := child(t, root, "concurrent")
		input, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		output, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill() })
		if line, err := bufio.NewReader(output).ReadString('\n'); err != nil || line != "ready\n" {
			t.Fatal("helper failed to start", err)
		}
		commands = append(commands, cmd)
		inputs = append(inputs, input)
	}
	for _, input := range inputs {
		if _, err := input.Write([]byte{1}); err != nil {
			t.Fatal(err)
		}
		if err := input.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	s := local(t, root, 1)
	assertBytes(t, s, identity("payload"), "payload")
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal("concurrent process cleanup", err)
	}
	if filepath.Base(entries[0].Name()) != identity("payload").SHA256 {
		t.Fatal("wrong persisted object")
	}
}

func TestRestrictiveProcessUmaskStillReturnsReadableObject(t *testing.T) {
	root := privateRoot(t)
	if err := child(t, root, "restrictive").Run(); err != nil {
		t.Fatal(err)
	}
	s := local(t, root, 1)
	assertBytes(t, s, identity("payload"), "payload")
}

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

func TestTrustedPutAndVerify(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "input")
	data := []byte("private material\n")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	common := []string{"--root", root, "--sha256", digest, "--size", "17", "--max-object-bytes", "128", "--timeout", "10s"}
	var output bytes.Buffer
	if err := run(context.Background(), append([]string{"put", "--source", source}, common...), &output); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(output.Bytes(), data) {
		t.Fatal("private payload exposed")
	}
	if err := run(context.Background(), append([]string{"put", "--source", source}, common...), failingOutput{}); !errors.Is(err, materialstore.ErrCommitUnknown) {
		t.Fatal("lost write acknowledgment", err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), append([]string{"verify"}, common...), io.Discard); err != nil {
		t.Fatal("reopen without original source", err)
	}
	if err := run(context.Background(), append([]string{"verify"}, common...), failingOutput{}); !errors.Is(err, io.ErrClosedPipe) || errors.Is(err, materialstore.ErrCommitUnknown) {
		t.Fatal("read-only verification must not imply a write attempt", err)
	}
}

type failingOutput struct{}

func (failingOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestExplicitCLIConfigurationAndSpecialSources(t *testing.T) {
	for _, args := range [][]string{nil, {"publish"}, {"put"}, {"verify", "--source", "x"}, {"verify", "--timeout", "0s"}, {"verify", "--unknown"}} {
		if err := run(context.Background(), args, io.Discard); err == nil {
			t.Fatal("invalid options accepted")
		}
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(filepath.Join(t.TempDir(), "nonexistent"), source); err != nil {
		t.Fatal(err)
	}
	args := []string{"put", "--source", source, "--root", root, "--sha256", hex.EncodeToString(make([]byte, 32)), "--size", "0", "--max-object-bytes", "128", "--timeout", "10s"}
	if err := run(context.Background(), args, io.Discard); err == nil {
		t.Fatal("symlink source accepted")
	}
}

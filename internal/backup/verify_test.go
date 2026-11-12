package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestVerifyTransfer(t *testing.T) {
	type mutation func(t *testing.T, f *transferFixture)
	for _, tc := range []struct {
		name, want string
		change     mutation
	}{
		{name: "matching read-only package", change: func(t *testing.T, f *transferFixture) {
			for _, path := range []string{manifestName, inventoryName, archiveName, filepath.Join(judgeDir, f.object), filepath.Join(privateDir, f.object)} {
				transferMust(t, os.Chmod(filepath.Join(f.root, path), 0400))
			}
			for _, path := range []string{f.root, filepath.Join(f.root, judgeDir), filepath.Join(f.root, privateDir)} {
				transferMust(t, os.Chmod(path, 0500))
				t.Cleanup(func() { transferMust(t, os.Chmod(path, 0700)) })
			}
		}},
		{name: "wrong trusted digest", want: "manifest SHA-256 did not match", change: func(_ *testing.T, f *transferFixture) {
			f.trusted = strings.Repeat("f", 64)
		}},
		{name: "partial package", want: "completion manifest unavailable", change: func(t *testing.T, f *transferFixture) {
			transferMust(t, os.Remove(filepath.Join(f.root, manifestName)))
		}},
		{name: "archive changed at the same length", want: "archive failed verification", change: func(t *testing.T, f *transferFixture) {
			f.write(t, archiveName, []byte("PGDMP_WRONG"))
		}},
		{name: "material changed at the same length", want: "material file failed verification", change: func(t *testing.T, f *transferFixture) {
			f.write(t, filepath.Join(privateDir, f.object), []byte("wr0ng"))
		}},
		{name: "missing material", want: "material file failed verification", change: func(t *testing.T, f *transferFixture) {
			transferMust(t, os.Remove(filepath.Join(f.root, judgeDir, f.object)))
		}},
		{name: "extra unlisted material", want: "unexpected or missing material entries", change: func(t *testing.T, f *transferFixture) {
			f.write(t, filepath.Join(judgeDir, strings.Repeat("f", 64)), []byte("extra"))
		}},
		{name: "extra top-level file", want: "unexpected or missing top-level entries", change: func(t *testing.T, f *transferFixture) {
			f.write(t, "extra", nil)
		}},
		{name: "inventory hash disagrees", want: "inventory failed verification", change: func(t *testing.T, f *transferFixture) {
			reversed := []inventoryEntry{f.entries[1], f.entries[0]}
			f.write(t, inventoryName, inventoryBytes(t, reversed))
		}},
		{name: "duplicate with matching inventory hash", want: "duplicate file", change: func(t *testing.T, f *transferFixture) {
			f.entries = append(f.entries, f.entries[0])
			f.manifest.JudgeFiles++
			f.manifest.References.JudgeReferences++
			f.seal(t)
		}},
		{name: "path-like digest", want: "inventory record invalid", change: func(t *testing.T, f *transferFixture) {
			f.entries[0].SHA256 = "../" + f.object[:61]
			f.seal(t)
		}},
		{name: "material symlink", want: "material file failed verification", change: func(t *testing.T, f *transferFixture) {
			path := filepath.Join(f.root, judgeDir, f.object)
			transferMust(t, os.Remove(path))
			transferMust(t, os.Symlink(filepath.Join("..", privateDir, f.object), path))
		}},
		{name: "material directory symlink", want: "directory must be private", change: func(t *testing.T, f *transferFixture) {
			transferMust(t, os.Remove(filepath.Join(f.root, judgeDir, f.object)))
			transferMust(t, os.Remove(filepath.Join(f.root, judgeDir)))
			transferMust(t, os.Symlink(privateDir, filepath.Join(f.root, judgeDir)))
		}},
		{name: "material FIFO", want: "material file failed verification", change: func(t *testing.T, f *transferFixture) {
			path := filepath.Join(f.root, privateDir, f.object)
			transferMust(t, os.Remove(path))
			transferMust(t, syscall.Mkfifo(path, 0600))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTransferFixture(t)
			tc.change(t, f)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			got, err := VerifyTransfer(ctx, f.root, f.trusted)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) || got != (TransferReceipt{}) {
					t.Fatalf("expected refusal %q with no receipt; got %+v (%v)", tc.want, got, err)
				}
				return
			}
			transferMust(t, err)
			if got != (TransferReceipt{ManifestSHA256: f.trusted, Archive: f.manifest.Archive, JudgeFiles: 1, PrivateFiles: 1}) {
				t.Fatalf("unexpected verification receipt: %+v", got)
			}
		})
	}
}

type transferFixture struct {
	root, object, trusted string
	entries               []inventoryEntry
	manifest              Manifest
}

func newTransferFixture(t *testing.T) *transferFixture {
	t.Helper()
	f := &transferFixture{root: t.TempDir()}
	transferMust(t, os.Chmod(f.root, 0700))
	object, archive := []byte("exact"), []byte("PGDMP_exact")
	f.object = testIdentity(object).SHA256
	for _, dir := range []string{judgeDir, privateDir} {
		transferMust(t, os.Mkdir(filepath.Join(f.root, dir), 0700))
		f.write(t, filepath.Join(dir, f.object), object)
		f.entries = append(f.entries, inventoryEntry{Store: dir, SHA256: f.object, Bytes: int64(len(object))})
	}
	f.write(t, archiveName, archive)
	f.manifest = Manifest{Format: backupFormat, Database: "fixture", Server: "14.24", CapturedAt: time.Unix(1700000000, 0).UTC(),
		Archive: testIdentity(archive), JudgeFiles: 1, PrivateFiles: 1}
	f.manifest.References.JudgeManifests = 1
	f.manifest.References.JudgeReferences = 1
	f.manifest.References.Drafts = 1
	f.manifest.References.PrivateReferences = 1
	f.seal(t)
	return f
}

func (f *transferFixture) write(t *testing.T, name string, data []byte) {
	t.Helper()
	transferMust(t, os.WriteFile(filepath.Join(f.root, name), data, 0600))
}

func (f *transferFixture) seal(t *testing.T) {
	t.Helper()
	index := inventoryBytes(t, f.entries)
	f.write(t, inventoryName, index)
	f.manifest.Inventory = testIdentity(index)
	raw, err := json.Marshal(f.manifest)
	transferMust(t, err)
	raw = append(raw, '\n')
	f.write(t, manifestName, raw)
	f.trusted = testIdentity(raw).SHA256
}

func inventoryBytes(t *testing.T, entries []inventoryEntry) []byte {
	t.Helper()
	var result []byte
	for _, entry := range entries {
		raw, err := json.Marshal(entry)
		transferMust(t, err)
		result = append(result, raw...)
		result = append(result, '\n')
	}
	return result
}

func testIdentity(raw []byte) FileIdentity {
	digest := sha256.Sum256(raw)
	return FileIdentity{SHA256: hex.EncodeToString(digest[:]), Bytes: int64(len(raw))}
}

func transferMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

package backup

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
)

const (
	maxManifestBytes = 16 << 10
	maxInventoryLine = 256
	judgeScope       = 1
	privateScope     = 2
)

// TransferReceipt confirms the exact package bytes without exposing source metadata.
type TransferReceipt struct {
	ManifestSHA256 string       `json:"manifest_sha256"`
	Archive        FileIdentity `json:"archive"`
	JudgeFiles     int64        `json:"judge_files"`
	PrivateFiles   int64        `json:"private_files"`
}

// VerifyTransfer checks a static received package against an independently
// obtained manifest digest. It does not connect to PostgreSQL or restore data.
func VerifyTransfer(ctx context.Context, path, expectedDigest string) (TransferReceipt, error) {
	var zero TransferReceipt
	if ctx == nil || !absoluteClean(path) || !validDigest(expectedDigest) {
		return zero, errors.New("backup verification requires an absolute path and a trusted lowercase manifest SHA-256")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return zero, errors.New("backup directory unavailable")
	}
	root, err := pinDirectory(info, func() (*os.Root, error) { return os.OpenRoot(path) })
	if err != nil {
		return zero, err
	}
	defer func() { _ = root.Close() }()
	manifest, err := readManifest(ctx, root, expectedDigest)
	if err != nil {
		return zero, err
	}
	if err := verifyRootNames(ctx, root); err != nil {
		return zero, err
	}
	if err := checkFile(ctx, root, archiveName, manifest.Archive, verifyArchive); err != nil {
		return zero, errors.New("backup archive failed verification")
	}
	if err := verifyInventory(ctx, root, manifest); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return TransferReceipt{ManifestSHA256: expectedDigest, Archive: manifest.Archive,
		JudgeFiles: manifest.JudgeFiles, PrivateFiles: manifest.PrivateFiles}, nil
}

func readManifest(ctx context.Context, root *os.Root, expected string) (Manifest, error) {
	var m Manifest
	file, info, err := openPackageFile(root, manifestName)
	if err != nil {
		return m, errors.New("backup completion manifest unavailable")
	}
	defer func() { _ = file.Close() }()
	if info.Size() > maxManifestBytes {
		return m, errors.New("backup manifest exceeds the size limit")
	}
	raw, err := io.ReadAll(io.LimitReader(cancelReader{ctx, file}, maxManifestBytes+1))
	if err != nil || len(raw) > maxManifestBytes {
		return m, errors.New("backup manifest could not be read")
	}
	actual := sha256.Sum256(raw)
	if hex.EncodeToString(actual[:]) != expected {
		return m, errors.New("backup manifest SHA-256 did not match")
	}
	if !decodeCanonicalLine(raw, &m) || !validManifest(m) {
		return Manifest{}, errors.New("backup manifest invalid or unsupported")
	}
	return m, nil
}

func validManifest(m Manifest) bool {
	r := m.References
	if m.Format != backupFormat || !namePattern.MatchString(m.Database) || m.CapturedAt.IsZero() || m.Server == "" || len(m.Server) > 256 ||
		!validDigest(m.Archive.SHA256) || m.Archive.Bytes < 5 || !validDigest(m.Inventory.SHA256) || m.Inventory.Bytes < 0 ||
		m.JudgeFiles < 0 || m.JudgeFiles > maxPackageFiles || m.PrivateFiles < 0 || m.PrivateFiles > maxPackageFiles-m.JudgeFiles ||
		r.JudgeManifests < 0 || r.JudgeReferences < m.JudgeFiles || r.Drafts < 0 || r.ReferencePlans < 0 ||
		r.DifferentialPlans < 0 || r.DifferentialPlans > r.ReferencePlans || r.PrivateReferences < m.PrivateFiles {
		return false
	}
	return m.Inventory.Bytes <= (m.JudgeFiles+m.PrivateFiles)*maxInventoryLine
}

// The writer produces canonical JSON lines. Exact round trip also rejects
// unknown fields, duplicate keys and trailing values without another parser.
func decodeCanonicalLine[T any](raw []byte, value *T) bool {
	if json.Unmarshal(raw, value) != nil {
		return false
	}
	canonical, err := json.Marshal(value)
	return err == nil && bytes.Equal(raw, append(canonical, '\n'))
}

func verifyArchive(reader io.Reader) error {
	var header [5]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil || string(header[:]) != "PGDMP" {
		return errors.New("not a PostgreSQL custom archive")
	}
	_, err := io.Copy(io.Discard, reader)
	return err
}

func verifyInventory(ctx context.Context, root *os.Root, manifest Manifest) error {
	judge, err := childDirectory(root, judgeDir)
	if err != nil {
		return err
	}
	defer func() { _ = judge.Close() }()
	private, err := childDirectory(root, privateDir)
	if err != nil {
		return err
	}
	defer func() { _ = private.Close() }()
	seen := make(map[[sha256.Size]byte]uint8)
	var judges, privates int64
	var entryErr error
	err = checkFile(ctx, root, inventoryName, manifest.Inventory, func(r io.Reader) error {
		reader := bufio.NewReaderSize(r, maxInventoryLine)
		for {
			line, err := reader.ReadSlice('\n')
			if err == io.EOF && len(line) == 0 {
				return nil
			}
			var item inventoryEntry
			if err != nil || !decodeCanonicalLine(line, &item) || !validDigest(item.SHA256) || item.Bytes < 0 {
				entryErr = errors.New("backup inventory record invalid")
				return entryErr
			}
			var scope uint8
			var store *os.Root
			switch {
			case item.Store == judgeDir && item.Bytes <= materials.MaxFileBytes && judges < manifest.JudgeFiles:
				scope, store = judgeScope, judge
				judges++
			case item.Store == privateDir && item.Bytes <= 128<<20 && privates < manifest.PrivateFiles:
				scope, store = privateScope, private
				privates++
			default:
				entryErr = errors.New("backup inventory store, size or count invalid")
				return entryErr
			}
			key := digestKey(item.SHA256)
			if seen[key]&scope != 0 {
				entryErr = errors.New("backup inventory contains a duplicate file")
				return entryErr
			}
			seen[key] |= scope
			if err := checkFile(ctx, store, item.SHA256, FileIdentity{SHA256: item.SHA256, Bytes: item.Bytes}, drain); err != nil {
				entryErr = errors.New("backup material file failed verification")
				return entryErr
			}
		}
	})
	if entryErr != nil {
		return entryErr
	}
	if err != nil {
		return errors.New("backup inventory failed verification")
	}
	if judges != manifest.JudgeFiles || privates != manifest.PrivateFiles {
		return errors.New("backup material counts did not match the manifest")
	}
	for _, dir := range []struct {
		root  *os.Root
		scope uint8
		count int64
	}{{judge, judgeScope, judges}, {private, privateScope, privates}} {
		if err := checkNames(ctx, dir.root, dir.count, func(name string) bool {
			return validDigest(name) && seen[digestKey(name)]&dir.scope != 0
		}); err != nil {
			return errors.New("backup contains unexpected or missing material entries")
		}
	}
	return nil
}

func validDigest(raw string) bool {
	if len(raw) != sha256.Size*2 {
		return false
	}
	for _, c := range raw {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func digestKey(raw string) [sha256.Size]byte {
	var key [sha256.Size]byte
	_, _ = hex.Decode(key[:], []byte(raw)) // All callers validate the digest first.
	return key
}

func drain(r io.Reader) error {
	_, err := io.Copy(io.Discard, r)
	return err
}

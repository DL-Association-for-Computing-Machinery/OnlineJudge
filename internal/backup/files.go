package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func checkOutput(output string, roots ...string) error {
	parent, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return errors.New("backup output parent unavailable")
	}
	actual := filepath.Join(parent, filepath.Base(output))
	for _, root := range roots {
		source, err := filepath.EvalSymlinks(root)
		if err != nil {
			return errors.New("backup source material root unavailable")
		}
		rel, err := filepath.Rel(source, actual)
		if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return errors.New("backup output must be outside the source material roots")
		}
	}
	return nil
}

func createOutput(output string) error {
	if err := os.Mkdir(output, 0700); err != nil {
		return errors.New("backup output must be a new directory")
	}
	for _, name := range []string{judgeDir, privateDir} {
		if err := os.Mkdir(filepath.Join(output, name), 0700); err != nil {
			return errors.New("backup output could not be prepared")
		}
	}
	return nil
}

func createFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
}

func writeIdentity(root, name string, write func(io.Writer) error) (FileIdentity, error) {
	file, err := createFile(filepath.Join(root, name))
	if err != nil {
		return FileIdentity{}, errors.New("backup output file could not be created")
	}
	defer func() { _ = file.Close() }()
	h := sha256.New()
	if err := write(io.MultiWriter(file, h)); err != nil {
		return FileIdentity{}, err
	}
	info, err := file.Stat()
	if err != nil || file.Sync() != nil || file.Close() != nil {
		return FileIdentity{}, errors.New("backup output file could not be synced")
	}
	return FileIdentity{SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: info.Size()}, nil
}

type inventoryEntry struct {
	Store  string `json:"store"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type materialFiles struct {
	output         string
	index          *json.Encoder
	judge, private int64
}

func (m *materialFiles) copy(store, digest string, expectedSize int64, transfer func(io.Writer) (int64, error)) error {
	path := filepath.Join(m.output, store, digest)
	file, err := createFile(path)
	if errors.Is(err, os.ErrExist) {
		// The output directory is new and private. Every previous successful
		// writer verified the bytes; a repeated private digest must agree in size.
		info, err := os.Lstat(path)
		if err != nil || info.Mode() != 0600 || (expectedSize >= 0 && info.Size() != expectedSize) {
			return errors.New("backup repeated file identity disagrees")
		}
		return nil
	}
	if err != nil {
		return errors.New("backup material file could not be created")
	}
	defer func() { _ = file.Close() }()
	if m.judge+m.private >= maxPackageFiles {
		return errors.New("backup exceeds the material file limit")
	}
	size, err := transfer(file)
	if err != nil {
		return err
	}
	if file.Sync() != nil || file.Close() != nil {
		return errors.New("backup material file could not be synced")
	}
	if err := m.index.Encode(inventoryEntry{Store: store, SHA256: digest, Bytes: size}); err != nil {
		return errors.New("backup material inventory could not be written")
	}
	if store == judgeDir {
		m.judge++
	} else {
		m.private++
	}
	return nil
}

func seal(output string, m Manifest) error {
	// Persist the owned directory and its completed files before making the
	// manifest visible. A failed pre-seal operation never writes manifest.json.
	for _, path := range []string{filepath.Join(output, judgeDir), filepath.Join(output, privateDir), output, filepath.Dir(output)} {
		if err := syncDir(path); err != nil {
			return errors.New("backup directory could not be synced")
		}
	}
	const staged = ".manifest.partial"
	if _, err := writeIdentity(output, staged, func(w io.Writer) error { return json.NewEncoder(w).Encode(m) }); err != nil {
		return errors.New("backup completion manifest could not be written")
	}
	if err := os.Link(filepath.Join(output, staged), filepath.Join(output, manifestName)); err != nil {
		return errors.New("backup completion manifest could not be published")
	}
	if err := errors.Join(os.Remove(filepath.Join(output, staged)), syncDir(output)); err != nil {
		return errors.New("backup seal durability unknown; verify the package before use")
	}
	return nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

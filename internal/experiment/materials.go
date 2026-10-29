package experiment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
)

// WriteMaterials explicitly initializes only the four public synthetic fixture
// blobs in an existing private directory. It never replaces an existing entry.
// A partial failure is safe to retry; conflicting or corrupt entries fail closed.
func WriteMaterials(path string) error {
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	// Cleanup does not change the completed operation or its primary error.
	defer func() { _ = root.Close() }()
	reader, err := materials.OpenLocal(path)
	if err != nil {
		return err
	}
	// Cleanup does not change the completed operation or its primary error.
	defer func() { _ = reader.Close() }()
	for _, data := range []string{"1 2\n", "3\n", "-7 4\n", "-3\n"} {
		h := sha256.Sum256([]byte(data))
		digest := hex.EncodeToString(h[:])
		if err := publish(root, digest, data); err != nil {
			return err
		}
		file, err := reader.Open(context.Background(), digest)
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

func publish(root *os.Root, digest, data string) error {
	name := ".fixture-" + rand.Text()
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(name) }() // Best-effort removal of the private staging link.
	// Cleanup does not change the completed operation or its primary error.
	defer func() { _ = file.Close() }()
	if _, err := file.WriteString(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	// Hard-link publication is atomic and cannot overwrite even a symlink.
	if err := root.Link(name, digest); err != nil && !os.IsExist(err) {
		return err
	}
	return nil
}

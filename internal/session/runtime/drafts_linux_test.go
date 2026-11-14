//go:build linux

package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestDraftStartupFailureReleasesPreparedAndMaterialRoots(t *testing.T) {
	for _, stage := range []string{"later-candidate", "repository-assembly"} {
		t.Run(stage, func(t *testing.T) {
			bundle := problemfixture.New(t)
			root := filepath.Join(t.TempDir(), "materials")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			lock := filepath.Join(t.TempDir(), "source-lock.json")
			if err := os.WriteFile(lock, bundle.Lock, 0600); err != nil {
				t.Fatal(err)
			}
			c := PrivateDraftsConfig{MaterialRoot: root, BundleDirectory: bundle.Directory, SourceLockFile: lock,
				SourceIDs: []string{"library-checker:aplusb"}, Timeout: time.Second, MaxInFlight: 1}
			if stage == "later-candidate" {
				c.SourceIDs = append(c.SourceIDs, "library-checker:unknown")
			}
			// Nil PG fails repository assembly only AFTER both Prepared and Local
			// have opened. The earlier case fails after the first candidate opened.
			i, cleanup, err := openDrafts(t.Context(), c, nil, nil, nil)
			if i != nil || cleanup != nil || !errors.Is(err, ErrStartup) {
				t.Fatal("partial startup unexpectedly succeeded")
			}
			entries, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				path, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
				if strings.HasPrefix(path, bundle.Directory) || strings.HasPrefix(path, root) {
					t.Fatal("failed startup retained a private directory descriptor")
				}
			}
		})
	}
}

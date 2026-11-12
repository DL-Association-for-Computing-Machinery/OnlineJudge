package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestTrialArchiveReadsOnlyCompletedRegularReport(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, dir string)
		ok    bool
	}{
		{"completed", func(t *testing.T, dir string) {
			writeTrialFile(t, dir, "report.json", []byte("evidence\n"))
		}, true},
		{"incomplete", func(t *testing.T, dir string) {
			writeTrialFile(t, dir, "report.json", []byte("evidence\n"))
			writeTrialFile(t, dir, "INCOMPLETE", nil)
		}, false},
		{"dangling marker", func(t *testing.T, dir string) {
			writeTrialFile(t, dir, "report.json", []byte("evidence\n"))
			if err := os.Symlink("absent", filepath.Join(dir, "INCOMPLETE")); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"report symlink", func(t *testing.T, dir string) {
			writeTrialFile(t, dir, "actual", []byte("evidence\n"))
			if err := os.Symlink("actual", filepath.Join(dir, "report.json")); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"report directory", func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, "report.json"), 0700); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"oversized report", func(t *testing.T, dir string) {
			writeTrialFile(t, dir, "report.json", make([]byte, problemvalidation.MaxCheckerTrialReportBytes+1))
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(t, dir)
			root, err := openTrialArchive(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = root.Close() }()
			got, err := readCompletedTrial(root)
			if tc.ok {
				if err != nil || !bytes.Equal(got, []byte("evidence\n")) {
					t.Fatalf("completed report not read: %v", err)
				}
			} else if err == nil || got != nil {
				t.Fatal("invalid archive yielded report bytes")
			}
		})
	}
}

func TestVerifyTrialRejectedArchiveDoesNotEmitSummary(t *testing.T) {
	dir := t.TempDir()
	writeTrialFile(t, dir, "report.json", []byte("evidence\n"))
	writeTrialFile(t, dir, "INCOMPLETE", nil)
	sha := strings.Repeat("a", 64)
	var out bytes.Buffer
	args := []string{"verify-trial", "--bundle", dir, "--source-lock", filepath.Join(dir, "lock"),
		"--report-dir", dir, "--expected-plan", sha, "--expected-candidate", sha, "--expected-report", sha,
		"--bytes", "262144", "--token-bytes", "12"}
	if err := run(t.Context(), args, &out, io.Discard); err == nil || out.Len() != 0 {
		t.Fatal("incomplete archive produced a successful command or stdout")
	}
	if _, err := os.Lstat(filepath.Join(dir, "INCOMPLETE")); err != nil {
		t.Fatal("offline verification modified the marker", err)
	}
}

func writeTrialFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

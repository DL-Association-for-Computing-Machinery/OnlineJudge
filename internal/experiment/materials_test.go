package experiment

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
)

func TestPublishedFixtureMatchesManifest(t *testing.T) {
	dir := t.TempDir()
	for range 2 {
		if err := WriteMaterials(dir); err != nil {
			t.Fatal(err)
		}
	}
	var manifest struct {
		Tests []struct {
			Input  string `json:"input_sha256"`
			Answer string `json:"answer_sha256"`
		} `json:"tests"`
	}
	if err := json.Unmarshal(Manifest(), &manifest); err != nil {
		t.Fatal(err)
	}
	reader, err := materials.OpenLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Cleanup does not change the completed operation or its primary error.
	defer func() { _ = reader.Close() }()
	want := [][]string{{"1 2\n", "3\n"}, {"-7 4\n", "-3\n"}}
	for i, test := range manifest.Tests {
		for j, hash := range []string{test.Input, test.Answer} {
			file, err := reader.Open(context.Background(), hash)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(file)
			_ = file.Close() // Cleanup; the operation outcome is checked separately.
			if err != nil || string(data) != want[i][j] {
				t.Fatalf("fixture bytes %q %v", data, err)
			}
		}
	}
	path := filepath.Join(dir, manifest.Tests[0].Input)
	if err := os.WriteFile(path, []byte("conflict"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteMaterials(dir); err == nil {
		t.Fatal("overwrote conflicting material")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "conflict" {
		t.Fatal("existing bytes changed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 4 {
		t.Fatal("publication leaked temp files")
	}
}

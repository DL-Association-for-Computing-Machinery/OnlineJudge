package materials

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
)

func TestLocalTextReaderDoesNotWidenLegacy(t *testing.T) {
	dir := t.TempDir()
	legacy, err := OpenLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = legacy.Close() }()
	local, err := OpenLocalText(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = local.Close() }()
	for _, n := range []int{MaxFileBytes + 1, LocalTextFileBytes, LocalTextFileBytes + 1} {
		data := bytes.Repeat([]byte("x"), n)
		hash := checksum(data)
		if err := os.WriteFile(filepath.Join(dir, hash), data, 0600); err != nil {
			t.Fatal(err)
		}
		if legacy.Verify(t.Context(), hash) == nil {
			t.Fatalf("legacy accepted %d bytes", n)
		}
		if (local.Verify(t.Context(), hash) == nil) != (n <= LocalTextFileBytes) {
			t.Fatalf("unexpected local limit for %d bytes", n)
		}
	}
}

func TestLocalSPJManifestRequiresExplicitArtifactAndMode(t *testing.T) {
	hash := checksum([]byte("fixture"))
	m := TextTransportManifest{Checker: LocalSPJChecker, CheckerArtifact: hash,
		JudgeSHA256: hash, RuntimeSHA256: hash, Origin: LocalTextOrigin,
		Language: "gnu++17", Version: "local-text-" + hash,
		Tests: []TextTransportPoint{{ID: "p1", Input: hash, Answer: hash}}}
	check := func(m TextTransportManifest, ok bool) {
		t.Helper()
		raw := m.Canonical()
		if _, err := ParseLocalText(raw, checksum(raw)); (err == nil) != ok {
			t.Fatalf("unexpected local checker admission: %v", err)
		}
	}
	check(m, true)
	m.TokenBytes = 4096
	check(m, false)
	m.TokenBytes = 0
	m.CheckerArtifact = ""
	check(m, false)
}

func TestLocalTextTransportImport(t *testing.T) {
	hash := strings.Repeat("a", 64)
	m := APlusBManifest{Origin: LocalTextOrigin, Version: "local-text-" + hash, JudgeSHA256: hash,
		RuntimeSHA256: hash, Language: "gnu++17", Checker: judging.LibraryCheckerTextVersion,
		TokenBytes: 4096, Tests: []APlusBPoint{{ID: "test-1", Input: hash, Answer: checksum(nil)}}}
	check := func(m APlusBManifest, expectedOK bool) {
		t.Helper()
		raw := m.Canonical()
		if _, err := ParseLocalText(raw, checksum(raw)); (err == nil) != expectedOK {
			t.Fatalf("valid=%t, error=%v", expectedOK, err)
		}
	}
	check(m, true)
	raw := m.Canonical()
	if _, err := ParseAPlusB(raw, checksum(raw)); err == nil {
		t.Fatal("legacy parser accepted local text")
	}
	if _, err := ParseLocalText(raw, hash); err == nil {
		t.Fatal("accepted wrong manifest digest")
	}
	duplicate := m
	duplicate.Tests = append(duplicate.Tests, duplicate.Tests[0])
	check(duplicate, false)
	m.Version = "local-text-invalid"
	check(m, false)
}

func TestLocalDecimalManifestRejectsImplicitOrMixedModes(t *testing.T) {
	hash := strings.Repeat("a", 64)
	m := TextTransportManifest{Checker: judging.DecimalVersion,
		Decimal: &judging.DecimalPolicy{Absolute: "0", Relative: "1e-3"},
		Origin:  LocalTextOrigin, Version: "local-text-" + hash, JudgeSHA256: hash,
		RuntimeSHA256: hash, Language: "gnu++17",
		Tests: []TextTransportPoint{{ID: "p1", Input: hash, Answer: hash}}}
	for _, tc := range []struct {
		name   string
		change func(*TextTransportManifest)
		valid  bool
	}{
		{"explicit", func(*TextTransportManifest) {}, true},
		{"missing", func(m *TextTransportManifest) { m.Decimal = nil }, false},
		{"negative", func(m *TextTransportManifest) { m.Decimal = &judging.DecimalPolicy{Relative: "-1"} }, false},
		{"mixed with text", func(m *TextTransportManifest) { m.Checker = judging.LibraryCheckerTextVersion; m.TokenBytes = 4096 }, false},
		{"mixed with SPJ", func(m *TextTransportManifest) { m.Checker = LocalSPJChecker; m.CheckerArtifact = hash }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := m
			tc.change(&copy)
			raw := copy.Canonical()
			if _, err := ParseLocalText(raw, checksum(raw)); (err == nil) != tc.valid {
				t.Fatalf("unexpected admission: %v", err)
			}
		})
	}
}

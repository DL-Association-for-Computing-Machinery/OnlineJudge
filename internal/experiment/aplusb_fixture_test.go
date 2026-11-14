package experiment

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
)

func TestLocalTextFixtureRequiresExplicitUbuntuPin(t *testing.T) {
	hash := strings.Repeat("a", 64)
	m := materials.APlusBManifest{Origin: materials.LocalTextOrigin, Version: "local-text-" + hash, JudgeSHA256: hash,
		RuntimeSHA256: APlusBRuntimeSHA256ForTier(ReferenceAPlusBUbuntu), Checker: judging.LibraryCheckerTextVersion,
		Language: "gnu++17", TokenBytes: 4096, Tests: []materials.APlusBPoint{{ID: "case-1", Input: hash, Answer: hash}}}
	for _, name := range []string{"valid", "wrong-profile", "wrong-pin", "mixed", "missing-opt-in"} {
		t.Run(name, func(t *testing.T) {
			manifest := m
			if name == "wrong-profile" {
				manifest.RuntimeSHA256 = APlusBRuntimeSHA256()
			}
			raw := manifest.Canonical()
			sum := sha256.Sum256(raw)
			path := filepath.Join(t.TempDir(), "manifest.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			env := map[string]string{"OJ_SANDBOX_EXPERIMENT": "1", "OJ_LOCAL_TEXT_MANIFEST_FILE": path,
				"OJ_LOCAL_TEXT_MANIFEST_SHA256": hex.EncodeToString(sum[:])}
			switch name {
			case "wrong-pin":
				env["OJ_LOCAL_TEXT_MANIFEST_SHA256"] = hash
			case "mixed":
				env["OJ_APLUSB_MANIFEST_FILE"] = path
			case "missing-opt-in":
				delete(env, "OJ_SANDBOX_EXPERIMENT")
			}
			_, fixture, err := LoadCachedFixture(func(k string) string { return env[k] })
			if (err == nil) != (name == "valid") || fixture.IsLocalText() != (name == "valid") {
				t.Fatalf("unexpected fixture result: %v", err)
			}
			if fixture.IsLocalText() && (fixture.Version() != m.Version || fixture.APlusBTier() != ReferenceAPlusBUbuntu) {
				t.Fatal("wrong runtime or material version")
			}
		})
	}
}

func TestAPlusBFixtureBindsCheckerAndRuntime(t *testing.T) {
	identity := strings.Repeat("a", 64)
	base := materials.APlusBManifest{Checker: judging.LibraryCheckerTextVersion, RuntimeSHA256: APlusBRuntimeSHA256(),
		JudgeSHA256: identity, Version: "aplusb-" + identity, Origin: materials.APlusBOrigin, Language: "gnu++17", TokenBytes: 12}
	for i := range 17 {
		base.Tests = append(base.Tests, materials.APlusBPoint{ID: fmt.Sprintf("case-%02d", i), Input: identity, Answer: identity})
	}
	check := func(m materials.APlusBManifest, want bool) {
		t.Helper()
		raw := m.Canonical()
		sha := sha256.Sum256(raw)
		fixture, err := APlusBFixture(raw, hex.EncodeToString(sha[:]))
		if (err == nil) != want || fixture.IsAPlusB() != want {
			t.Fatalf("unexpected fixture admission: %v", err)
		}
	}
	check(base, true)
	bad := base
	bad.RuntimeSHA256 = strings.Repeat("f", 64)
	check(bad, false)
	bad = base
	bad.Checker = "ascii-tokens-v1"
	check(bad, false)
}

func TestAPlusBConfigurationRejectsCrossRuntimeManifests(t *testing.T) {
	if APlusBRuntimeSHA256() != "dfe2ee521a3102051494301c829524105991c030732f15afb910803723461725" {
		t.Fatal("existing Desktop runtime pin changed")
	}
	identity := strings.Repeat("a", 64)
	manifest := materials.APlusBManifest{Checker: judging.LibraryCheckerTextVersion,
		JudgeSHA256: identity, Version: "aplusb-" + identity, Origin: materials.APlusBOrigin, Language: "gnu++17", TokenBytes: 12}
	for i := range 17 {
		manifest.Tests = append(manifest.Tests, materials.APlusBPoint{ID: fmt.Sprintf("case-%02d", i), Input: identity, Answer: identity})
	}
	for _, target := range []struct {
		setting string
		tier    ReferenceTier
		kernel  string
	}{
		{"", ReferenceSmall, "7.0.14-linuxkit"},
		{"ubuntu-arm64", ReferenceAPlusBUbuntu, "6.8.0-106-generic"},
		{"", ReferenceAPlusBTrialAMD64, ""},
	} {
		manifest.RuntimeSHA256 = APlusBRuntimeSHA256ForTier(target.tier)
		candidate := target.tier == ReferenceAPlusBTrialAMD64
		if candidate {
			if manifest.RuntimeSHA256 != "" {
				t.Fatal("offline candidate exposed by executable runtime lookup")
			}
			if _, err := ResolveReferenceTier(target.tier); err == nil {
				t.Fatal("offline candidate admitted for execution")
			}
			manifest.RuntimeSHA256 = APlusBTrialAMD64CandidateSHA256()
		}
		raw := manifest.Canonical()
		hash := sha256.Sum256(raw)
		path := filepath.Join(t.TempDir(), "manifest.json")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		env := map[string]string{
			"OJ_SANDBOX_EXPERIMENT": "1", "OJ_APLUSB_MANIFEST_FILE": path,
			"OJ_APLUSB_MANIFEST_SHA256": hex.EncodeToString(hash[:]),
		}
		for _, setup := range []string{"", "ubuntu-arm64", "trial-amd64-candidate", "unsupported"} {
			env["OJ_APLUSB_RUNTIME"] = setup
			_, f, err := LoadAPlusBFixture(func(k string) string { return env[k] })
			if candidate || setup != target.setting {
				if err == nil || f.IsAPlusB() {
					t.Fatal("incorrect runtime admitted a manifest")
				}
			} else if err != nil || f.APlusBTier() != target.tier || f.Profile().KernelRelease != target.kernel {
				t.Fatalf("pinned runtime unavailable: %v", err)
			}
		}
	}
}

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
)

func materialDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func TestRejudgeRequiresPinnedSPJArtifactAndPointBudget(t *testing.T) {
	root := t.TempDir()
	put := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	point := []byte("1\n")
	hash := materialDigest(point)
	put(hash, point)
	artifact := []byte("fixture checker identity")
	id := strings.Repeat("b", 64)
	m := materials.TextTransportManifest{Origin: materials.LocalTextOrigin, Version: "local-text-" + id, JudgeSHA256: id,
		RuntimeSHA256: experiment.LocalSPJRuntimeSHA256(), Checker: materials.LocalSPJChecker,
		CheckerArtifact: materialDigest(artifact), Language: "gnu++17",
		Tests: []materials.TextTransportPoint{{ID: "case-1", Input: hash, Answer: hash}}}
	env := map[string]string{"OJ_SANDBOX_EXPERIMENT": "1", "OJ_MATERIAL_ROOT": root}
	check := func(want bool) {
		t.Helper()
		raw := m.Canonical()
		env["OJ_LOCAL_TEXT_MANIFEST_FILE"] = put("manifest.json", raw)
		env["OJ_LOCAL_TEXT_MANIFEST_SHA256"] = materialDigest(raw)
		err := verifyRejudgeMaterialWithEnv(t.Context(), m.Version, materialDigest(raw), raw, func(k string) string { return env[k] })
		if (err == nil) != want {
			t.Fatalf("rejudge admission success=%v, want %v (error: %v)", err == nil, want, err)
		}
	}
	check(false)
	env["OJ_LOCAL_SPJ_CHECKER_FILE"] = put("checker", []byte("different artifact"))
	check(false)
	put("checker", artifact)
	check(true)
	large := make([]byte, experiment.SPJObjectBytes+1)
	m.Tests[0].Input = materialDigest(large)
	put(m.Tests[0].Input, large)
	check(false)
}

func TestRejudgeUsesPinnedLocalTextMaterials(t *testing.T) {
	root := t.TempDir()
	rawInput := make([]byte, materials.MaxFileBytes+1)
	object := materialDigest(rawInput)
	objectPath := filepath.Join(root, object)
	if err := os.WriteFile(objectPath, rawInput, 0600); err != nil {
		t.Fatal(err)
	}
	answer := materialDigest(nil)
	if err := os.WriteFile(filepath.Join(root, answer), nil, 0600); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 64)
	m := materials.TextTransportManifest{Origin: materials.LocalTextOrigin, Version: "local-text-" + id, JudgeSHA256: id,
		RuntimeSHA256: experiment.APlusBRuntimeSHA256ForTier(experiment.ReferenceAPlusBUbuntu), Checker: judging.LibraryCheckerTextVersion,
		Language: "gnu++17", TokenBytes: 4096, Tests: []materials.TextTransportPoint{{ID: "case-1", Input: object, Answer: answer}}}
	raw := m.Canonical()
	pin := materialDigest(raw)
	path := filepath.Join(root, "manifest.json")
	env := map[string]string{"OJ_MATERIAL_ROOT": root, "OJ_SANDBOX_EXPERIMENT": "1",
		"OJ_LOCAL_TEXT_MANIFEST_FILE": path, "OJ_LOCAL_TEXT_MANIFEST_SHA256": pin}
	check := func(want bool, version, digest string, manifest []byte) {
		t.Helper()
		err := verifyRejudgeMaterialWithEnv(t.Context(), version, digest, manifest, func(k string) string { return env[k] })
		if (err == nil) != want {
			t.Fatalf("rejudge admission success=%v, want %v (error: %v)", err == nil, want, err)
		}
	}
	writePin := func(raw []byte) {
		t.Helper()
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		env["OJ_LOCAL_TEXT_MANIFEST_SHA256"] = materialDigest(raw)
	}
	writePin(raw)
	check(true, m.Version, pin, raw)
	check(false, m.Version, id, raw)
	check(false, "different-version", pin, raw)
	check(false, m.Version, pin, append(raw, '\n'))

	env["OJ_SANDBOX_EXPERIMENT"] = ""
	check(false, m.Version, pin, raw)
	env["OJ_SANDBOX_EXPERIMENT"] = "1"
	env["OJ_APLUSB_RUNTIME"] = "ubuntu-arm64"
	check(false, m.Version, pin, raw)
	delete(env, "OJ_APLUSB_RUNTIME")
	wrongTier := m
	wrongTier.RuntimeSHA256 = experiment.APlusBRuntimeSHA256()
	writePin(wrongTier.Canonical())
	check(false, m.Version, materialDigest(wrongTier.Canonical()), wrongTier.Canonical())
	alternate := m
	alternate.TokenBytes++
	writePin(alternate.Canonical())
	check(false, m.Version, pin, raw)
	badAnswer := m
	badAnswer.Tests = []materials.TextTransportPoint{{ID: "case-1", Input: object, Answer: object}}
	writePin(badAnswer.Canonical())
	check(false, m.Version, materialDigest(badAnswer.Canonical()), badAnswer.Canonical())
	overTotal := m
	overTotal.Tests = nil
	for i := range 48 {
		overTotal.Tests = append(overTotal.Tests, materials.TextTransportPoint{ID: fmt.Sprintf("case-%d", i), Input: object, Answer: answer})
	}
	writePin(overTotal.Canonical())
	check(false, m.Version, materialDigest(overTotal.Canonical()), overTotal.Canonical())
	writePin(raw)

	if err := os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	check(false, m.Version, pin, raw)
	rawInput[0] = 1
	if err := os.WriteFile(objectPath, rawInput, 0600); err != nil {
		t.Fatal(err)
	}
	check(false, m.Version, pin, raw)
}

func TestRejudgeRetainsHistoricalFixtureAdmission(t *testing.T) {
	unset := func(string) string { return "" }
	for _, version := range []string{experiment.PinnedVersion, experiment.UbuntuVersion, experiment.Version} {
		_, raw, err := experiment.SelectFixture(version)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rejudgeFixture(version, materialDigest(raw), raw, unset); (err == nil) != (version != experiment.Version) {
			t.Fatalf("unexpected historical admission for %s: %v", version, err)
		}
	}
	id := strings.Repeat("a", 64)
	m := materials.APlusBManifest{Origin: materials.APlusBOrigin, Version: "aplusb-" + id, JudgeSHA256: id,
		Checker: judging.LibraryCheckerTextVersion, Language: "gnu++17", TokenBytes: 4096}
	for i := range 17 {
		m.Tests = append(m.Tests, materials.APlusBPoint{ID: fmt.Sprintf("case-%d", i), Input: id, Answer: id})
	}
	m.RuntimeSHA256 = experiment.APlusBRuntimeSHA256()
	raw := m.Canonical()
	if _, err := rejudgeFixture(m.Version, materialDigest(raw), raw, unset); err != nil {
		t.Fatalf("legacy Desktop contract stopped accepting its historical manifest: %v", err)
	}
	m.RuntimeSHA256 = experiment.APlusBRuntimeSHA256ForTier(experiment.ReferenceAPlusBUbuntu)
	raw = m.Canonical()
	digest := materialDigest(raw)
	if _, err := rejudgeFixture(m.Version, digest, raw, unset); err == nil {
		t.Fatal("Ubuntu runtime admitted without trusted configuration")
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"OJ_SANDBOX_EXPERIMENT": "1", "OJ_APLUSB_RUNTIME": "ubuntu-arm64",
		"OJ_APLUSB_MANIFEST_FILE": path, "OJ_APLUSB_MANIFEST_SHA256": digest}
	if _, err := rejudgeFixture(m.Version, digest, raw, func(k string) string { return env[k] }); err != nil {
		t.Fatalf("pinned Ubuntu A+B stopped being eligible: %v", err)
	}
}

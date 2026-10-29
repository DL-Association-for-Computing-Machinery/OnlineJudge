package experiment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestVersionedEnvironment(t *testing.T) {
	version, legacy, err := SelectFixture("")
	if err != nil || version != Version || !bytes.Equal(legacy, Manifest()) {
		t.Fatal("legacy default changed")
	}
	legacySum := sha256.Sum256(legacy)
	if hex.EncodeToString(legacySum[:]) != "5825e71b429c1b4b41d853f714bd8e078f3f48adf06232f0903d5717d1703dea" {
		t.Fatal("v1 immutable manifest changed")
	}
	var old, pinned map[string]any
	if err := json.Unmarshal(legacy, &old); err != nil {
		t.Fatal(err)
	}
	if old["execution_environment"] != "unvalidated" {
		t.Fatal("legacy environment relabeled")
	}
	version, current, err := SelectFixture(PinnedVersion)
	if err != nil || version == Version {
		t.Fatal("missing separate version")
	}
	_, v2, err := SelectFixture("m0-sum-v2")
	v2Sum := sha256.Sum256(v2)
	if err != nil || hex.EncodeToString(v2Sum[:]) != "4699ed93798241b796c5c00701f7150e1d8c80fb7e3ba536b9344cf88fa3d68a" {
		t.Fatalf("v2 immutable manifest changed (%x)", v2Sum)
	}
	if version != "m0-sum-v3" || bytes.Equal(v2, current) {
		t.Fatal("Desktop migration requires distinct v3 material")
	}
	if err := json.Unmarshal(current, &pinned); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(old["tests"], pinned["tests"]) || pinned["checker"] != old["checker"] {
		t.Fatal("problem/checker changed")
	}
	if _, ok := pinned["execution_environment"].(map[string]any); !ok {
		t.Fatal("profile absent")
	}
	p := PinnedEnvironment()
	p.CompileArgs[0] = "modified"
	p.Env[0] = "modified"
	_, again, _ := SelectFixture(PinnedVersion)
	if !bytes.Equal(current, again) {
		t.Fatal("caller mutated shared profile")
	}
	for _, value := range []string{"latest", "m0-sum-v4", " m0-sum-v3", "M0-SUM-V2"} {
		if _, _, err := SelectFixture(value); err == nil {
			t.Fatalf("unknown selection %q accepted", value)
		}
	}
}

func TestPinnedBuildRecipe(t *testing.T) {
	data, err := os.ReadFile("../../deploy/experiments/go-judge.Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != PinnedEnvironment().DockerfileSHA256 {
		t.Fatal("recipe changed: build and verify a new image/profile/material version")
	}
}

package experiment

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
)

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

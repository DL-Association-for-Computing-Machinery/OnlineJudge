package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestAPlusBTargetHandoffReceiptCLI(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "archived-receipt")
	// Use the existing handoff fixture's pinned target, with synthetic digests
	// in place of actual received reports.
	target := problemvalidation.APlusBHandoffTarget{
		Node: "local-rehearsal", Revision: "4184ed4c01297704f702b349c1f4b2e7cb7f4a35",
		Runtime: "1dacc3c77e44446d598490dd0d808ff32e1bcd1114e715af521406fcc7804ef6", Slot: "oj-reference-sandbox",
	}
	handoff := "ef36b6a9d5b06369448910972fa32acc6a25b32b839bd77e384540bb85484d85"
	pin, err := problemvalidation.SealAPlusBHandoffReceipt(t.Context(), path, problemvalidation.APlusBHandoffReadback{
		Scope: "report-readback-only", Target: target, Points: 17, Handoff: handoff,
		Reports: [3]string{strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)},
	})
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"aplusb-target-handoff", "--mode", "receipt", "--directory", path,
		"--node", target.Node, "--revision", target.Revision, "--runtime", target.Runtime, "--slot", target.Slot,
		"--expected-handoff", handoff}
	var out, diagnostics bytes.Buffer
	for _, extra := range [][]string{nil, {"--expected-receipt", strings.Repeat("0", 64)}, {"--expected-receipt", pin, "--receipt-out", path}} {
		if err := run(t.Context(), append(args, extra...), &out, &diagnostics); err == nil || out.Len() != 0 {
			t.Fatal("readback printed a receipt without the exact independent inputs")
		}
	}
	if err := run(t.Context(), append(args, "--expected-receipt", pin), &out, &diagnostics); err != nil ||
		!strings.Contains(out.String(), "\"scope\":\"report-readback-only\"") ||
		!strings.Contains(out.String(), "\"judge_manifest_sha256\":") {
		t.Fatalf("matched receipt did not read back its scope and manifest: %v", err)
	}
}

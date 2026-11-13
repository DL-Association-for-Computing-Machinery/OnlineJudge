package problemvalidation

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This fixture represents a synthetic, already verified readback; it makes no
// claim that a node was contacted or that the report bytes exist.
func receiptReadback(t *testing.T) APlusBHandoffReadback {
	t.Helper()
	target := APlusBHandoffTarget{"fixture-node", strings.Repeat("a", 40), ubuntuHandoffRuntime, "oj-aplusb-ubuntu-sandbox"}
	handoff, err := handoffFor(target)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(handoff)
	if err != nil {
		t.Fatal(err)
	}
	return APlusBHandoffReadback{
		Scope: "report-readback-only", Handoff: handoffDigest(raw), Target: target, Points: 17,
		Reports: [3]string{strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)},
	}
}

func TestAPlusBHandoffReceiptSealAndRead(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "new")
	r := receiptReadback(t)
	pin, err := SealAPlusBHandoffReceipt(t.Context(), path, r)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadAPlusBHandoffReceipt(path, pin, r.Handoff, r.Target)
	if err != nil || got.ImportRevision != "e96d280c89a6e5d3da76ee5347c0581d4153d7052e9afceddffb2fdbb3d2036d" ||
		got.Binding.Candidate != ReviewedAPlusBCandidate || got.Binding.Judge != ubuntuHandoffJudge ||
		got.Binding.Transport != ubuntuHandoffTransport || got.Readback != r {
		t.Fatalf("receipt lost its frozen binding: %v", err)
	}
	changed := r
	changed.Reports[1] = strings.Repeat("4", 64)
	if _, err := SealAPlusBHandoffReceipt(t.Context(), path, changed); err == nil {
		t.Fatal("existing receipt was writable")
	}
	if _, err := ReadAPlusBHandoffReceipt(path, pin, r.Handoff, r.Target); err != nil {
		t.Fatal("original receipt was lost")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	unwritten := filepath.Join(base, "cancelled")
	if _, err := SealAPlusBHandoffReceipt(cancelled, unwritten, r); err == nil {
		t.Fatal("cancelled seal returned a digest")
	}
	if _, err := os.Stat(unwritten); !os.IsNotExist(err) {
		t.Fatal("cancelled seal created a directory")
	}
	if err := os.Chmod(filepath.Join(path, handoffReceiptFile), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAPlusBHandoffReceipt(path, pin, r.Handoff, r.Target); err == nil {
		t.Fatal("read accepted a receipt that was not private")
	}
}

func TestAPlusBHandoffReceiptRejectsWrongBindingEvenWithMatchingDigest(t *testing.T) {
	r := receiptReadback(t)
	raw, err := encodeAPlusBHandoffReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	pin := handoffDigest(raw)
	other := r.Target
	other.Node = "other-node"
	for _, test := range []struct {
		name         string
		body         []byte
		pin, handoff string
		target       APlusBHandoffTarget
	}{
		{"different node", raw, pin, r.Handoff, other},
		{"different handoff", raw, pin, strings.Repeat("4", 64), r.Target},
		{"different receipt pin", raw, strings.Repeat("4", 64), r.Handoff, r.Target},
		{"rewritten revision", bytes.Replace(raw, []byte(reviewedAPlusBImport().ImportRevision), []byte(strings.Repeat("4", 64)), 1), "", r.Handoff, r.Target},
		{"claims admission", bytes.Replace(raw, []byte("report-readback-only"), []byte("target-runtime-admitted"), 1), "", r.Handoff, r.Target},
		{"unknown field", bytes.Replace(raw, []byte("{"), []byte("{\"target_runtime_admitted\":true,"), 1), "", r.Handoff, r.Target},
		{"extra bytes", append(bytes.Clone(raw), '\n'), "", r.Handoff, r.Target},
	} {
		t.Run(test.name, func(t *testing.T) {
			expected := test.pin
			if expected == "" {
				expected = handoffDigest(test.body)
			}
			if _, err := verifyAPlusBHandoffReceipt(test.body, expected, test.handoff, test.target); err == nil {
				t.Fatal("receipt accepted")
			}
		})
	}
	for name, reports := range map[string][3]string{
		"known local": {reviewedAPlusBReports[0].report, r.Reports[1], r.Reports[2]},
		"duplicate":   {r.Reports[0], r.Reports[0], r.Reports[2]},
	} {
		t.Run(name, func(t *testing.T) {
			changed := r
			changed.Reports = reports
			if _, err := encodeAPlusBHandoffReceipt(changed); err == nil {
				t.Fatal("invalid report combination accepted")
			}
		})
	}
}

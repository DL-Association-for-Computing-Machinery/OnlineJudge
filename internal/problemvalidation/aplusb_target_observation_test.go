package problemvalidation

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func targetObservationFixture(t *testing.T) (APlusBHandoffReceipt, string, string, aplusbTargetObservation) {
	t.Helper()
	readback := receiptReadback(t)
	receipt, err := aplusbReceiptFor(readback)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := encodeAPlusBHandoffReceipt(readback)
	if err != nil {
		t.Fatal(err)
	}
	receiptPin, worker := handoffDigest(raw), strings.Repeat("a", 64)
	return receipt, receiptPin, worker, aplusbTargetObservation{
		Schema: aplusbTargetObservationSchema, Material: observationMaterial(receipt),
		Target: readback.Target, WorkerArtifact: worker, BuildProvenance: strings.Repeat("b", 64),
		Handoff: readback.Handoff, Receipt: receiptPin, Reports: readback.Reports,
		Observed:       aplusbObservationProvenance{"test-operator", "2026-11-14T03:04:00Z", strings.Repeat("c", 64)},
		WorkerVerified: true, NodeVerified: true, IsolationChecked: true, Stopped: true,
	}
}

func TestAPlusBTargetObservationBindsReceiptAndExpectedArtifact(t *testing.T) {
	receipt, receiptPin, worker, good := targetObservationFixture(t)
	if err := verifyAPlusBTargetObservation(good, receipt, receiptPin, worker); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*aplusbTargetObservation)
	}{
		{"receipt kind", func(o *aplusbTargetObservation) { o.Schema = handoffReceiptSchema }},
		{"same content version changed import", func(o *aplusbTargetObservation) { o.Material.ImportRevision = strings.Repeat("4", 64) }},
		{"wrong node", func(o *aplusbTargetObservation) { o.Target.Node = "other-node" }},
		{"wrong Git revision", func(o *aplusbTargetObservation) { o.Target.Revision = strings.Repeat("4", 40) }},
		{"wrong slot", func(o *aplusbTargetObservation) { o.Target.Slot = "other-slot" }},
		{"wrong runtime", func(o *aplusbTargetObservation) { o.Target.Runtime = strings.Repeat("4", 64) }},
		{"wrong worker bytes", func(o *aplusbTargetObservation) { o.WorkerArtifact = strings.Repeat("4", 64) }},
		{"missing build provenance", func(o *aplusbTargetObservation) { o.BuildProvenance = "" }},
		{"wrong handoff", func(o *aplusbTargetObservation) { o.Handoff = strings.Repeat("4", 64) }},
		{"wrong receipt", func(o *aplusbTargetObservation) { o.Receipt = strings.Repeat("4", 64) }},
		{"wrong order", func(o *aplusbTargetObservation) { o.Reports[0], o.Reports[1] = o.Reports[1], o.Reports[0] }},
		{"missing observation", func(o *aplusbTargetObservation) { o.Observed.Evidence = "" }},
		{"invalid observation time", func(o *aplusbTargetObservation) { o.Observed.ObservedAt = "2026-02-30T09:00:00Z" }},
		{"self-declared worker", func(o *aplusbTargetObservation) { o.WorkerVerified = false }},
		{"self-declared node", func(o *aplusbTargetObservation) { o.NodeVerified = false }},
		{"no isolation check", func(o *aplusbTargetObservation) { o.IsolationChecked = false }},
		{"no stop confirmation", func(o *aplusbTargetObservation) { o.Stopped = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := good
			tc.change(&changed)
			if err := verifyAPlusBTargetObservation(changed, receipt, receiptPin, worker); err == nil {
				t.Fatal("observation with a different or incomplete binding accepted")
			}
		})
	}
}

func TestAPlusBObservationCanonicalFormat(t *testing.T) {
	_, _, _, good := targetObservationFixture(t)
	raw, err := json.Marshal(good)
	if err != nil {
		t.Fatal(err)
	}
	canonical := append(raw, '\n')
	if got, err := decodeAPlusBObservation[aplusbTargetObservation](canonical, handoffDigest(canonical)); err != nil || got != good {
		t.Fatalf("canonical observation: %v", err)
	}
	for _, body := range [][]byte{
		bytes.Replace(canonical, []byte("{"), []byte("{\"private_path\":\"redacted\","), 1),
		bytes.Replace(canonical, []byte("{"), []byte("{\"schema\":\"other\","), 1),
		bytes.Replace(canonical, []byte("\"ac_wa_ole_report_sha256\""), []byte("\"AC_WA_OLE_REPORT_SHA256\""), 1),
		bytes.Replace(canonical, []byte("\"ac_wa_ole_report_sha256\":["), []byte("\"ac_wa_ole_report_sha256\":[\"additional\","), 1),
		bytes.Join([][]byte{canonical, []byte("{}")}, nil),
		bytes.Repeat([]byte(" "), maxAPlusBObservationBytes+1),
	} {
		if _, err := decodeAPlusBObservation[aplusbTargetObservation](body, handoffDigest(body)); err == nil {
			t.Fatal("noncanonical observation accepted even with matching digest")
		}
	}
	if _, err := decodeAPlusBObservation[aplusbTargetObservation](canonical, strings.Repeat("d", 64)); err == nil {
		t.Fatal("observation accepted with wrong external digest")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "observation.json")
	if err := os.WriteFile(path, canonical, 0600); err != nil {
		t.Fatal(err)
	}
	pin := handoffDigest(canonical)
	if got, err := readAPlusBObservation[aplusbTargetObservation](path, pin); err != nil || got != good {
		t.Fatalf("private observation did not read back: %v", err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readAPlusBObservation[aplusbTargetObservation](path, pin); err == nil {
		t.Fatal("accepted a file without private permissions")
	}
}

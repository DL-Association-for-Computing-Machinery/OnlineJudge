package problemvalidation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

const (
	handoffReceiptSchema   = "oj.aplusb-target-report-receipt.v1"
	handoffReadbackScope   = "report-readback-only"
	handoffReceiptFile     = "receipt.json"
	maxHandoffReceiptBytes = 4096
)

// APlusBHandoffReceipt describes a report readback, with no assertion that the
// declared node and build are authentic or that a production gate is satisfied.
type APlusBHandoffReceipt struct {
	Schema           string                `json:"schema"`
	SourceID         string                `json:"source_id"`
	SourceLock       string                `json:"source_lock_sha256"`
	ContentVersion   string                `json:"content_version"`
	ImportRevision   string                `json:"import_revision"`
	ReferenceProfile string                `json:"reference_profile_sha256"`
	Binding          APlusBHandoff         `json:"binding"`
	Readback         APlusBHandoffReadback `json:"readback"`
}

func aplusbReceiptFor(readback APlusBHandoffReadback) (APlusBHandoffReceipt, error) {
	var zero APlusBHandoffReceipt
	h, err := handoffFor(readback.Target)
	if err != nil || h.Schema != handoffSchema || readback.Scope != handoffReadbackScope || readback.Points != 17 {
		return zero, errHandoff
	}
	encoded, _ := json.Marshal(h)
	if handoffDigest(encoded) != readback.Handoff {
		return zero, errHandoff
	}
	for i, digest := range readback.Reports {
		if !problem.ValidImportRevision(digest) || knownAPlusBLocalReport(digest) {
			return zero, errHandoff
		}
		for _, prior := range readback.Reports[:i] {
			if digest == prior {
				return zero, errHandoff
			}
		}
	}
	id := reviewedAPlusBImport()
	return APlusBHandoffReceipt{
		Schema: handoffReceiptSchema, SourceID: id.SourceID, SourceLock: id.SourceLockSHA256,
		ContentVersion: id.Version, ImportRevision: id.ImportRevision, ReferenceProfile: reviewedAPlusBReferenceProfile,
		Binding: h, Readback: readback,
	}, nil
}

func encodeAPlusBHandoffReceipt(readback APlusBHandoffReadback) ([]byte, error) {
	r, err := aplusbReceiptFor(readback)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// SealAPlusBHandoffReceipt writes to a new directory under a trusted parent.
// Call after verifying the handoff, pinned reports, and completion markers.
func SealAPlusBHandoffReceipt(ctx context.Context, path string, readback APlusBHandoffReadback) (string, error) {
	raw, err := encodeAPlusBHandoffReceipt(readback)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.Mkdir(path, 0700); err != nil {
		return "", errHandoff
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return "", errHandoff
	}
	defer func() { _ = root.Close() }()
	if err := writeHandoffFile(root, handoffReceiptFile, func(w io.Writer) error {
		_, err := w.Write(raw)
		return err
	}); err != nil {
		return "", errHandoff
	}
	if err := syncHandoffRoot(root); err != nil {
		return "", errHandoff
	}
	parent, err := os.Open(filepath.Dir(path))
	if err != nil {
		return "", errHandoff
	}
	if err := errors.Join(parent.Sync(), parent.Close(), ctx.Err()); err != nil {
		return "", errHandoff
	}
	return handoffDigest(raw), nil
}

// ReadAPlusBHandoffReceipt checks the exact received bytes and independently
// selected target and handoff. It does not reverify reports or attest a run.
func ReadAPlusBHandoffReceipt(path, pin, handoff string, target APlusBHandoffTarget) (APlusBHandoffReceipt, error) {
	raw, err := checkedfile.ReadSecret(filepath.Join(path, handoffReceiptFile), maxHandoffReceiptBytes)
	if err != nil {
		return APlusBHandoffReceipt{}, errHandoff
	}
	return verifyAPlusBHandoffReceipt(raw, pin, handoff, target)
}

func verifyAPlusBHandoffReceipt(raw []byte, pin, handoff string, target APlusBHandoffTarget) (APlusBHandoffReceipt, error) {
	var r, zero APlusBHandoffReceipt
	if len(raw) == 0 || len(raw) > maxHandoffReceiptBytes || handoffDigest(raw) != pin ||
		json.Unmarshal(raw, &r) != nil || r.Readback.Target != target || r.Readback.Handoff != handoff {
		return zero, errHandoff
	}
	expected, err := encodeAPlusBHandoffReceipt(r.Readback)
	if err != nil || !bytes.Equal(raw, expected) {
		return zero, errHandoff
	}
	return r, nil
}

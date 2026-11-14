package problemvalidation

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

const (
	aplusbTargetObservationSchema = "oj.aplusb-target-observation.v1"
	maxAPlusBObservationBytes     = 4096
	observationTimeFormat         = "2006-01-02T15:04:05Z"
)

var errAPlusBObservation = errors.New("A+B observation is missing or does not match the independently selected binding")

type aplusbObservationMaterial struct {
	SourceID         string `json:"source_id"`
	SourceLock       string `json:"source_lock_sha256"`
	ContentVersion   string `json:"content_version"`
	ImportRevision   string `json:"import_revision"`
	ReferenceProfile string `json:"reference_profile_sha256"`
	FormalPlan       string `json:"formal_plan_sha256"`
	Candidate        string `json:"candidate_sha256"`
}

func observationMaterial(r APlusBHandoffReceipt) aplusbObservationMaterial {
	return aplusbObservationMaterial{
		SourceID: r.SourceID, SourceLock: r.SourceLock, ContentVersion: r.ContentVersion,
		ImportRevision: r.ImportRevision, ReferenceProfile: r.ReferenceProfile,
		FormalPlan: r.Binding.Plan, Candidate: r.Binding.Candidate,
	}
}

type aplusbObservationProvenance struct {
	Observer   string `json:"observer"`
	ObservedAt string `json:"observed_at"`
	Evidence   string `json:"evidence_sha256"`
}

func (p aplusbObservationProvenance) valid() bool {
	at, err := time.Parse(observationTimeFormat, p.ObservedAt)
	return err == nil && at.Year() > 1 && at.UTC().Format(observationTimeFormat) == p.ObservedAt &&
		handoffLabel.MatchString(p.Observer) && problem.ValidImportRevision(p.Evidence)
}

type aplusbTargetObservation struct {
	Schema           string                      `json:"schema"`
	Material         aplusbObservationMaterial   `json:"material"`
	Target           APlusBHandoffTarget         `json:"target"`
	WorkerArtifact   string                      `json:"worker_artifact_sha256"`
	BuildProvenance  string                      `json:"build_provenance_sha256"`
	Handoff          string                      `json:"handoff_sha256"`
	Receipt          string                      `json:"receipt_sha256"`
	Reports          [3]string                   `json:"ac_wa_ole_report_sha256"`
	Observed         aplusbObservationProvenance `json:"observation"`
	WorkerVerified   bool                        `json:"worker_and_revision_observed"`
	NodeVerified     bool                        `json:"node_and_runtime_observed"`
	IsolationChecked bool                        `json:"isolation_checked"`
	Stopped          bool                        `json:"stop_confirmed"`
}

func readAPlusBObservation[T any](path, pin string) (T, error) {
	var zero T
	raw, err := checkedfile.ReadSecret(path, maxAPlusBObservationBytes)
	if err != nil {
		return zero, errAPlusBObservation
	}
	return decodeAPlusBObservation[T](raw, pin)
}

// Check canonical bytes after parsing. This rejects repeated or unknown keys,
// loose field-name aliases and fixed arrays that Go would pad or truncate.
func decodeAPlusBObservation[T any](raw []byte, pin string) (T, error) {
	var v, zero T
	if len(raw) == 0 || len(raw) > maxAPlusBObservationBytes ||
		!problem.ValidImportRevision(pin) || handoffDigest(raw) != pin || json.Unmarshal(raw, &v) != nil {
		return zero, errAPlusBObservation
	}
	canonical, err := json.Marshal(v)
	if err != nil || !bytes.Equal(raw, append(canonical, '\n')) {
		return zero, errAPlusBObservation
	}
	return v, nil
}

// The receipt must come from the pinned handoff reader. The Worker digest and
// receipt pin are supplied separately; do not derive either from the observation.
func verifyAPlusBTargetObservation(o aplusbTargetObservation, r APlusBHandoffReceipt, receiptPin, worker string) error {
	if o.Schema != aplusbTargetObservationSchema || o.Material != observationMaterial(r) ||
		o.Target != r.Binding.Target || o.Handoff != r.Readback.Handoff || o.Receipt != receiptPin ||
		o.Reports != r.Readback.Reports || !problem.ValidImportRevision(worker) || o.WorkerArtifact != worker ||
		!problem.ValidImportRevision(o.BuildProvenance) || !o.Observed.valid() ||
		!o.WorkerVerified || !o.NodeVerified || !o.IsolationChecked || !o.Stopped {
		return errAPlusBObservation
	}
	return nil
}

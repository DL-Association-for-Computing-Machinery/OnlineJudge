package problemvalidation

import (
	"context"
	"slices"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

const aplusbCheckerObservationSchema = "oj.aplusb-checker-observation.v1"

type aplusbCheckerObservation struct {
	Schema         string                      `json:"schema"`
	Material       aplusbObservationMaterial   `json:"material"`
	Target         APlusBHandoffTarget         `json:"target"`
	WorkerArtifact string                      `json:"worker_artifact_sha256"`
	CheckerVersion string                      `json:"checker_version"`
	OutputBytes    int                         `json:"output_bytes"`
	TokenBytes     int                         `json:"token_bytes"`
	TargetRecord   string                      `json:"target_observation_sha256"`
	Observed       aplusbObservationProvenance `json:"observation"`
	ReferenceOK    bool                        `json:"reference_accepted"`
	WrongOutputOK  bool                        `json:"wrong_output_rejected"`
	BoundedCapture bool                        `json:"bounded_capture_verified"`
	FaultsChecked  bool                        `json:"fault_classification_verified"`
}

func verifyAPlusBCheckerObservation(o aplusbCheckerObservation, target aplusbTargetObservation, pin string) error {
	limits := ReviewedAPlusBLimits()
	if o.Schema != aplusbCheckerObservationSchema || o.Material != target.Material ||
		o.Target != target.Target || o.WorkerArtifact != target.WorkerArtifact ||
		o.CheckerVersion != judging.LibraryCheckerTextVersion || o.OutputBytes != limits.Bytes ||
		o.TokenBytes != limits.TokenBytes || o.TargetRecord != pin ||
		!o.Observed.valid() || o.Observed.Evidence == target.Observed.Evidence ||
		slices.Contains(target.Reports[:], o.Observed.Evidence) ||
		!o.ReferenceOK || !o.WrongOutputOK || !o.BoundedCapture || !o.FaultsChecked {
		return errAPlusBObservation
	}
	return nil
}

// APlusBAdmissionInputs identifies private records and independent expectations.
// Every digest and target field must be obtained outside the received files.
type APlusBAdmissionInputs struct {
	ReceiptDirectory, ReceiptSHA256, HandoffSHA256   string
	TargetObservationPath, TargetObservationSHA256   string
	CheckerObservationPath, CheckerObservationSHA256 string
	WorkerArtifactSHA256                             string
	Target                                           APlusBHandoffTarget
}

// APlusBAdmissionReadback contains only the fixed binding for human review,
// without observation notes or any authority to change an approval gate.
type APlusBAdmissionReadback struct {
	Scope                    string              `json:"scope"`
	ImportRevision           string              `json:"import_revision"`
	FormalPlanSHA256         string              `json:"formal_plan_sha256"`
	CandidateSHA256          string              `json:"candidate_sha256"`
	Target                   APlusBHandoffTarget `json:"target"`
	WorkerArtifactSHA256     string              `json:"worker_artifact_sha256"`
	HandoffSHA256            string              `json:"handoff_sha256"`
	ReceiptSHA256            string              `json:"receipt_sha256"`
	TargetObservationSHA256  string              `json:"target_observation_sha256"`
	CheckerObservationSHA256 string              `json:"checker_observation_sha256"`
}

// ReadAPlusBAdmissionEvidence only verifies consistency of independently
// pinned inputs. It cannot attest provenance, timing or actual operation.
func ReadAPlusBAdmissionEvidence(ctx context.Context, in APlusBAdmissionInputs) (APlusBAdmissionReadback, error) {
	var zero APlusBAdmissionReadback
	for _, pin := range [...]string{in.ReceiptSHA256, in.HandoffSHA256, in.WorkerArtifactSHA256,
		in.TargetObservationSHA256, in.CheckerObservationSHA256} {
		if !problem.ValidImportRevision(pin) {
			return zero, errAPlusBObservation
		}
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	receipt, err := ReadAPlusBHandoffReceipt(in.ReceiptDirectory, in.ReceiptSHA256, in.HandoffSHA256, in.Target)
	if err != nil {
		return zero, errAPlusBObservation
	}
	target, err := readAPlusBObservation[aplusbTargetObservation](in.TargetObservationPath, in.TargetObservationSHA256)
	if err != nil || verifyAPlusBTargetObservation(target, receipt, in.ReceiptSHA256, in.WorkerArtifactSHA256) != nil {
		return zero, errAPlusBObservation
	}
	checker, err := readAPlusBObservation[aplusbCheckerObservation](in.CheckerObservationPath, in.CheckerObservationSHA256)
	if err != nil || verifyAPlusBCheckerObservation(checker, target, in.TargetObservationSHA256) != nil {
		return zero, errAPlusBObservation
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return APlusBAdmissionReadback{
		Scope: "review-input-only", ImportRevision: receipt.ImportRevision, FormalPlanSHA256: receipt.Binding.Plan,
		CandidateSHA256: receipt.Binding.Candidate, Target: in.Target, WorkerArtifactSHA256: in.WorkerArtifactSHA256,
		HandoffSHA256: in.HandoffSHA256, ReceiptSHA256: in.ReceiptSHA256,
		TargetObservationSHA256: in.TargetObservationSHA256, CheckerObservationSHA256: in.CheckerObservationSHA256,
	}, nil
}

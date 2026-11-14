package problemvalidation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

// APlusBLocalAdmissionInput is a trusted local operator configuration.
type APlusBLocalAdmissionInput struct {
	HandoffDirectory  string              `json:"handoff_directory"`
	HandoffSHA256     string              `json:"handoff_sha256"`
	Target            APlusBHandoffTarget `json:"target"`
	ReportDirectories [3]string           `json:"report_directories"`
	ReportSHA256      [3]string           `json:"report_sha256"`
}

// APlusBLocalAdmission is sealed by the report verifier. It represents only
// a local functional run, and conveys no authority over production.
type APlusBLocalAdmission struct {
	digest, version, transport string
	raw                        string
}

// Valid checks the lexical bounds before any filesystem access.
func (in APlusBLocalAdmissionInput) Valid() bool {
	binding, err := handoffFor(in.Target)
	if err != nil || binding.Target.Runtime != ubuntuHandoffRuntime || !problem.ValidImportRevision(in.HandoffSHA256) {
		return false
	}
	for i, dir := range in.ReportDirectories {
		if !absoluteLocalPath(dir) || !problem.ValidImportRevision(in.ReportSHA256[i]) {
			return false
		}
	}
	return absoluteLocalPath(in.HandoffDirectory)
}

func absoluteLocalPath(path string) bool {
	return len(path) <= 4000 && filepath.IsAbs(path) && filepath.Clean(path) == path
}

// ReadAPlusBLocalAdmission rechecks the transport and all three actual report
// bodies against operator pins. Receipt-only and unadmitted profiles fail.
func ReadAPlusBLocalAdmission(ctx context.Context, in APlusBLocalAdmissionInput) (APlusBLocalAdmission, error) {
	var zero APlusBLocalAdmission
	if ctx == nil || !in.Valid() {
		return zero, errHandoff
	}
	var raw [3][]byte
	for i, dir := range in.ReportDirectories {
		if _, err := os.Lstat(filepath.Join(dir, "INCOMPLETE")); !errors.Is(err, os.ErrNotExist) {
			return zero, errHandoff
		}
		var err error
		raw[i], err = checkedfile.ReadSecret(filepath.Join(dir, "report.json"), MaxCheckerTrialReportBytes)
		if err != nil {
			return zero, errHandoff
		}
	}
	readback, err := VerifyAPlusBHandoffReports(ctx, in.HandoffDirectory, in.HandoffSHA256, in.Target, raw, in.ReportSHA256)
	if err != nil {
		return zero, err
	}
	h, err := handoffFor(in.Target)
	if err != nil {
		return zero, err
	}
	transport, err := readTransport(in.HandoffDirectory, h)
	if err != nil {
		return zero, err
	}
	for _, dir := range in.ReportDirectories {
		if _, err := os.Lstat(filepath.Join(dir, "INCOMPLETE")); !errors.Is(err, os.ErrNotExist) {
			return zero, errHandoff
		}
	}
	binding, err := json.Marshal(readback)
	if err != nil || ctx.Err() != nil {
		return zero, errHandoff
	}
	return APlusBLocalAdmission{handoffDigest(binding), "aplusb-" + h.Judge, h.Transport, string(transport.raw)}, nil
}

// Match returns the local proof only for the exact previously reviewed A+B evidence.
func (a APlusBLocalAdmission) Match(id problem.ImportIdentity, plan, profile string) bool {
	return a.digest != "" && matchesReviewedApPlusBPlan(id, plan, profile)
}

// Binding gives the proof identity and immutable registered execution tuple.
func (a APlusBLocalAdmission) Binding() (proof, version, execution string) {
	return a.digest, a.version, a.transport
}

// Manifest returns a detached copy for explicit maintenance registration.
func (a APlusBLocalAdmission) Manifest() []byte { return []byte(a.raw) }

// ReviewBinding returns the frozen import and formal plan for a verified run.
func (a APlusBLocalAdmission) ReviewBinding() (problem.ImportIdentity, string) {
	if a.digest == "" {
		return problem.ImportIdentity{}, ""
	}
	return reviewedAPlusBImport(), ReviewedAPlusBPlan
}

// ValidateManifest checks the id, profile and digest of the pinned transport.
func (a APlusBLocalAdmission) ValidateManifest() bool {
	m, err := materials.ParseAPlusB(a.Manifest(), a.transport)
	return a.digest != "" && err == nil && m.Version == a.version
}

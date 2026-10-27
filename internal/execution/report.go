package execution

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

var leaseToken = regexp.MustCompile(`^[0-9a-f]{64}$`)

// JudgeReport runs a supervised attempt and freezes its report after cleanup.
// On any execution, lease, cancellation or stop error it returns no report; the
// controller's bounded recovery handles the abandoned attempt. Keep the returned
// value and original token for retries: rerunning code produces different evidence.
func (r *LeasedRunner) JudgeReport(ctx context.Context, lease submission.Lease, d materials.Descriptor, blobs map[string][]byte) (submission.Result, error) {
	if !leaseToken.MatchString(lease.Token) || lease.SubmissionID == "" || lease.SubmissionID != d.SubmissionID ||
		lease.JudgeGeneration < 1 || lease.RecoveryGeneration < 1 || lease.AttemptNumber < 1 {
		return submission.Result{}, errors.New("invalid attempt attribution")
	}
	result, err := r.Judge(ctx, lease.Token, d, blobs)
	if err != nil {
		return submission.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return submission.Result{}, err
	}
	return freezeReport(lease, d, result), nil
}

// Only locally produced, completed results reach this encoder. Explicit fields
// keep credentials, source, answers, raw output and ephemeral artifact IDs out of
// evidence. The manifest digest binds environment, checker and ordered tests.
func freezeReport(lease submission.Lease, d materials.Descriptor, result Result) submission.Result {
	type compilation struct {
		Status          string
		Exit            int
		CPU, Memory     int64
		OutputTruncated bool
	}
	evidence := struct {
		Schema                                                  string
		SubmissionID                                            string
		JudgeGeneration, RecoveryGeneration                     int64
		AttemptNumber                                           int32
		MaterialVersion, Language, ManifestSHA256, SourceSHA256 string
		Verdict                                                 string
		Compile                                                 compilation
		Points                                                  []Point
		Max                                                     *Usage
	}{Schema: "m0-execution-evidence-v1", SubmissionID: lease.SubmissionID,
		JudgeGeneration: lease.JudgeGeneration, RecoveryGeneration: lease.RecoveryGeneration,
		AttemptNumber: lease.AttemptNumber, MaterialVersion: d.Version, Language: d.Language,
		ManifestSHA256: d.ManifestSHA256, SourceSHA256: d.SourceSHA256,
		Verdict: result.Verdict, Points: result.Points, Max: result.Max}
	if c := result.Compile; c != nil {
		evidence.Compile = compilation{c.Status, c.Exit, c.CPU, c.Memory, c.OutputTruncated}
	}
	data, _ := json.Marshal(evidence) // fixed structs contain no fallible JSON values
	return submission.Result{Verdict: result.Verdict, EvidenceSHA256: digest(data)}
}

package problemvalidation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
)

func handoffDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

const handoffSchema = "oj.aplusb-target-handoff.v1"
const handoffManifest = "handoff.json"
const handoffTransport = "manifest.json"
const transportPin = "d896b91d12727acc290e98f6cd8b0944d590efa905efaa1c84ef7cbe4a2f4a13"

var errHandoff = errors.New("A+B handoff is incomplete or does not match the trusted binding")
var handoffLabel = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
var handoffRevision = regexp.MustCompile(`^[a-f0-9]{40}$`)

// APlusBHandoffTarget specifies the intended recipient and the independently
// selected runtime report profile. It is a request, not a node attestation.
type APlusBHandoffTarget struct {
	Node     string `json:"node"`
	Revision string `json:"revision"`
	Runtime  string `json:"runtime_profile_sha256"`
	Slot     string `json:"slot"`
}

// APlusBHandoff binds only the requested run and the fixed material identities.
type APlusBHandoff struct {
	Schema    string              `json:"schema"`
	Target    APlusBHandoffTarget `json:"target"`
	Plan      string              `json:"formal_plan_sha256"`
	Candidate string              `json:"candidate_sha256"`
	Judge     string              `json:"judge_manifest_sha256"`
	Transport string              `json:"transport_sha256"`
}

func handoffFor(target APlusBHandoffTarget) (APlusBHandoff, error) {
	if !handoffLabel.MatchString(target.Node) ||
		!handoffRevision.MatchString(target.Revision) || target.Runtime != reviewedAPlusBRuntime ||
		target.Slot != "oj-reference-sandbox" {
		return APlusBHandoff{}, errHandoff
	}
	return APlusBHandoff{handoffSchema, target, ReviewedAPlusBPlan, ReviewedAPlusBCandidate, reviewedAPlusBManifest, transportPin}, nil
}

func readTransport(path string) ([]byte, []string, error) {
	raw, err := checkedfile.ReadSecret(filepath.Join(path, handoffTransport), 16384)
	if err != nil {
		return nil, nil, errHandoff
	}
	m, err := materials.ParseAPlusB(raw, transportPin)
	if err != nil {
		return nil, nil, errHandoff
	}
	seen := make(map[string]bool)
	var objects []string
	for _, point := range m.Tests {
		for _, h := range []string{point.Input, point.Answer} {
			if !seen[h] {
				objects = append(objects, h)
				seen[h] = true
			}
		}
	}
	sort.Strings(objects)
	return raw, objects, nil
}

// PackAPlusBHandoff copies only the independently pinned transport and its
// referenced objects. The parent must be trusted; use only a new output path.
func PackAPlusBHandoff(ctx context.Context, source, destination string, target APlusBHandoffTarget) (string, error) {
	h, err := handoffFor(target)
	if err != nil {
		return "", err
	}
	raw, objects, err := readTransport(source)
	if err != nil {
		return "", err
	}
	reader, err := materials.OpenLocal(source)
	if err != nil {
		return "", errHandoff
	}
	defer func() { _ = reader.Close() }()
	if err := os.Mkdir(destination, 0700); err != nil {
		return "", errHandoff
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return "", errHandoff
	}
	defer func() { _ = root.Close() }()
	for _, hash := range objects {
		if err := writeHandoffFile(root, hash, func(out io.Writer) error {
			_, err := reader.CopyVerified(ctx, hash, out)
			return err
		}); err != nil {
			return "", errHandoff
		}
	}
	if err := writeHandoffFile(root, handoffTransport, func(out io.Writer) error { _, err := out.Write(raw); return err }); err != nil {
		return "", errHandoff
	}
	if err := errors.Join(syncHandoffRoot(root), ctx.Err()); err != nil {
		return "", err
	}
	encoded, _ := json.Marshal(h)
	if err := writeHandoffFile(root, handoffManifest, func(out io.Writer) error { _, err := out.Write(encoded); return err }); err != nil {
		return "", errHandoff
	}
	if err := syncHandoffRoot(root); err != nil {
		return "", err
	}
	parent, err := os.Open(filepath.Dir(destination))
	if err != nil {
		return "", err
	}
	if err := errors.Join(parent.Sync(), parent.Close(), ctx.Err()); err != nil {
		return "", err
	}
	return handoffDigest(encoded), nil
}

func writeHandoffFile(root *os.Root, name string, write func(io.Writer) error) error {
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	return errors.Join(write(f), f.Sync(), f.Close())
}

func syncHandoffRoot(root *os.Root) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

// VerifyAPlusBHandoff checks a stationary received copy against an independent
// manifest pin and the expected target. It does not execute or authorize it.
func VerifyAPlusBHandoff(ctx context.Context, path, pin string, target APlusBHandoffTarget) (APlusBHandoff, error) {
	var zero APlusBHandoff
	want, err := handoffFor(target)
	if err != nil {
		return zero, err
	}
	raw, err := checkedfile.ReadSecret(filepath.Join(path, handoffManifest), 4096)
	expected, _ := json.Marshal(want)
	if err != nil || handoffDigest(raw) != pin || !bytes.Equal(raw, expected) {
		return zero, errHandoff
	}
	_, objects, err := readTransport(path)
	if err != nil {
		return zero, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return zero, errHandoff
	}
	defer func() { _ = root.Close() }()
	dir, err := root.Open(".")
	if err != nil {
		return zero, errHandoff
	}
	names, err := dir.Readdirnames(len(objects) + 3)
	if closeErr := dir.Close(); (err != nil && !errors.Is(err, io.EOF)) || closeErr != nil {
		return zero, errHandoff
	}
	allowed := append(append([]string(nil), objects...), handoffManifest, handoffTransport)
	sort.Strings(names)
	sort.Strings(allowed)
	if len(names) != len(allowed) {
		return zero, errHandoff
	}
	for i, name := range names {
		info, err := root.Lstat(name)
		if name != allowed[i] || err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return zero, errHandoff
		}
	}
	reader, err := materials.OpenLocal(path)
	if err != nil {
		return zero, errHandoff
	}
	defer func() { _ = reader.Close() }()
	for _, hash := range objects {
		if err := reader.Verify(ctx, hash); err != nil {
			return zero, errHandoff
		}
	}
	return want, ctx.Err()
}

// APlusBHandoffReadback records a byte and report-shape check. The report
// digests must come from the designated operator through a trusted channel.
type APlusBHandoffReadback struct {
	Scope   string              `json:"scope"`
	Handoff string              `json:"handoff_sha256"`
	Target  APlusBHandoffTarget `json:"target"`
	Reports [3]string           `json:"ac_wa_ole_report_sha256"`
	Points  int                 `json:"points"`
}

// VerifyAPlusBHandoffReports checks the received bundle and three independently
// pinned reports. No signature, node attestation or approval is inferred.
func VerifyAPlusBHandoffReports(ctx context.Context, path, pin string, target APlusBHandoffTarget, reports [3][]byte, digests [3]string) (APlusBHandoffReadback, error) {
	var zero APlusBHandoffReadback
	h, err := VerifyAPlusBHandoff(ctx, path, pin, target)
	if err != nil {
		return zero, err
	}
	raw, _, err := readTransport(path)
	if err != nil {
		return zero, err
	}
	m, err := materials.ParseAPlusB(raw, h.Transport)
	if err != nil {
		return zero, errHandoff
	}
	for i, raw := range reports {
		p := reviewedAPlusBReports[i]
		if p.report == digests[i] {
			return zero, errHandoff
		}
		p.report = digests[i]
		if !matchesHandoffReport(raw, p, h, m.Tests) {
			return zero, errHandoff
		}
	}
	return APlusBHandoffReadback{"report-readback-only", pin, target, digests, len(m.Tests)}, ctx.Err()
}

func matchesHandoffReport(raw []byte, pin aplusbTrialPin, h APlusBHandoff, points []materials.APlusBPoint) bool {
	if len(raw) == 0 || len(raw) > MaxCheckerTrialReportBytes || handoffDigest(raw) != pin.report {
		return false
	}
	var r CheckerTrialReport
	if json.Unmarshal(raw, &r) != nil {
		return false
	}
	canonical, err := json.MarshalIndent(r, "", "  ")
	if err != nil || !bytes.Equal(raw, append(canonical, '\n')) ||
		r.Schema != "oj.aplusb-isolated-judge-report.v1" || r.CandidateSHA256 != h.Candidate ||
		r.JudgeManifestSHA256 != h.Judge || r.FormalPlanSHA != h.Plan || r.SubmissionSHA256 != pin.submission ||
		r.RuntimeSHA256 != h.Target.Runtime || r.RuntimeProfile.Digest() != h.Target.Runtime ||
		r.Slot != h.Target.Slot || r.Verdict != pin.verdict || !r.Complete || !r.StopConfirmed ||
		r.Diagnostic != nil || r.StopDiagnostic != nil || r.RuntimeProfile.Text != ReviewedAPlusBLimits() ||
		!recordedUsage(r.CompileUsage, r.RuntimeProfile.Runtime.Compile) || len(r.Points) != len(points) {
		return false
	}
	for i, point := range r.Points {
		want := pin.verdict
		if i > 0 && want != "AC" {
			want = "NOT_RUN"
		}
		if point.ID != points[i].ID || point.Verdict != want {
			return false
		}
		if want == "NOT_RUN" {
			if point.Usage != nil {
				return false
			}
		} else if !recordedUsage(point.Usage, r.RuntimeProfile.Runtime.Run) {
			return false
		}
	}
	return true
}

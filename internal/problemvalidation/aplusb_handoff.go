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

const (
	ubuntuHandoffRuntime   = "8cf3b8386627d1a92b7566521c78dd030bf00fdd2acd5485dcc9bc4357e492b8"
	ubuntuHandoffJudge     = "a2aac16ff12078b51106b6a26ed374aa89dbe386152742d6c1aeee2f189983bb"
	ubuntuHandoffTransport = "337c7a52705e4a8ff77c8d8ee03388563c3b54e80ed1b34b1d2a72e0863e4640"
)

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
	if !handoffLabel.MatchString(target.Node) || !handoffRevision.MatchString(target.Revision) {
		return APlusBHandoff{}, errHandoff
	}
	h := APlusBHandoff{Schema: handoffSchema, Target: target, Plan: ReviewedAPlusBPlan, Candidate: ReviewedAPlusBCandidate}
	switch {
	case target.Runtime == reviewedAPlusBRuntime && target.Slot == "oj-reference-sandbox":
		h.Judge, h.Transport = reviewedAPlusBManifest, transportPin
	case target.Runtime == ubuntuHandoffRuntime && target.Slot == "oj-aplusb-ubuntu-sandbox":
		h.Judge, h.Transport = ubuntuHandoffJudge, ubuntuHandoffTransport
	default:
		return APlusBHandoff{}, errHandoff
	}
	return h, nil
}

type handoffMaterials struct {
	raw     []byte
	points  []materials.APlusBPoint
	objects []string
}

func readTransport(path string, h APlusBHandoff) (handoffMaterials, error) {
	raw, err := checkedfile.ReadSecret(filepath.Join(path, handoffTransport), 16384)
	if err != nil {
		return handoffMaterials{}, errHandoff
	}
	m, err := materials.ParseAPlusB(raw, h.Transport)
	if err != nil || m.JudgeSHA256 != h.Judge {
		return handoffMaterials{}, errHandoff
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
	return handoffMaterials{raw, m.Tests, objects}, nil
}

// PackAPlusBHandoff copies only the independently pinned transport and its
// referenced objects. The parent must be trusted; use only a new output path.
func PackAPlusBHandoff(ctx context.Context, source, destination string, target APlusBHandoffTarget) (string, error) {
	h, err := handoffFor(target)
	if err != nil {
		return "", err
	}
	material, err := readTransport(source, h)
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
	for _, hash := range material.objects {
		if err := writeHandoffFile(root, hash, func(out io.Writer) error {
			_, err := reader.CopyVerified(ctx, hash, out)
			return err
		}); err != nil {
			return "", errHandoff
		}
	}
	if err := writeHandoffFile(root, handoffTransport, func(out io.Writer) error { _, err := out.Write(material.raw); return err }); err != nil {
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
	h, _, err := verifyAPlusBHandoff(ctx, path, pin, target)
	return h, err
}

func verifyAPlusBHandoff(ctx context.Context, path, pin string, target APlusBHandoffTarget) (APlusBHandoff, []materials.APlusBPoint, error) {
	var zero APlusBHandoff
	want, err := handoffFor(target)
	if err != nil {
		return zero, nil, err
	}
	raw, err := checkedfile.ReadSecret(filepath.Join(path, handoffManifest), 4096)
	expected, _ := json.Marshal(want)
	if err != nil || handoffDigest(raw) != pin || !bytes.Equal(raw, expected) {
		return zero, nil, errHandoff
	}
	material, err := readTransport(path, want)
	if err != nil {
		return zero, nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return zero, nil, errHandoff
	}
	defer func() { _ = root.Close() }()
	dir, err := root.Open(".")
	if err != nil {
		return zero, nil, errHandoff
	}
	objects := material.objects
	names, err := dir.Readdirnames(len(objects) + 3)
	if closeErr := dir.Close(); (err != nil && !errors.Is(err, io.EOF)) || closeErr != nil {
		return zero, nil, errHandoff
	}
	allowed := append(append([]string(nil), objects...), handoffManifest, handoffTransport)
	sort.Strings(names)
	sort.Strings(allowed)
	if len(names) != len(allowed) {
		return zero, nil, errHandoff
	}
	for i, name := range names {
		info, err := root.Lstat(name)
		if name != allowed[i] || err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return zero, nil, errHandoff
		}
	}
	reader, err := materials.OpenLocal(path)
	if err != nil {
		return zero, nil, errHandoff
	}
	defer func() { _ = reader.Close() }()
	for _, hash := range objects {
		if err := reader.Verify(ctx, hash); err != nil {
			return zero, nil, errHandoff
		}
	}
	if err := ctx.Err(); err != nil {
		return zero, nil, err
	}
	return want, material.points, nil
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
	h, points, err := verifyAPlusBHandoff(ctx, path, pin, target)
	if err != nil {
		return zero, err
	}
	for i, raw := range reports {
		p := reviewedAPlusBReports[i]
		if knownAPlusBLocalReport(digests[i]) {
			return zero, errHandoff
		}
		p.report = digests[i]
		if !matchesHandoffReport(raw, p, h, points) {
			return zero, errHandoff
		}
	}
	return APlusBHandoffReadback{handoffReadbackScope, pin, target, digests, len(points)}, ctx.Err()
}

// Known local runs are excluded; an independent operator pin is still required
// since a report contains neither node identity nor a proof of when it was run.
func knownAPlusBLocalReport(digest string) bool {
	for _, p := range reviewedAPlusBReports {
		if digest == p.report {
			return true
		}
	}
	switch digest {
	// Desktop handoff rehearsal.
	case "b5911a1ddbe56f9af2b54637a31bc5adaf58f79ea704dc7bcb0c4391f0f9a813",
		"2a98e4b7342ae82731688f824f12c16c93109a1d5c0ec5fd05108c4a8d3a3007",
		"b305c0b2f7045efc36c8c14c2516c47c633c12e03ff893ec971578671ad43d03":
		return true
	// Original Ubuntu trial and repeat from the final reviewed head.
	case "2ba03e32c76500ff8b9186098a529107993d458723dd28463dd3091abffb30d0",
		"f0a22987537b3e7715b3c12672aede16bc0e07f3aac3c14051329259d4d2a7af",
		"69746d200ea2845b04993199e8194a0f69d36e0255a8563f952087b0208901e5",
		"bbf58cfe552c7da6616681f8fcc231a58f8df490093133bdd3f2891c1505acd1",
		"79cda165f4e2687f9e733ee1f5b7f8549411ddc20a194f69bc725cca4b35845d",
		"e601560a16b15a9c3ed56291811e55ce450d730691e9c844aadcee8d9b6577b5":
		return true
	default:
		return false
	}
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

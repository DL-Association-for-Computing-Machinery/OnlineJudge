package workerclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// ErrMaterials means preparation failed or is unsupported. No partial material
// set may be executed or reported as a contestant verdict.
var ErrMaterials = errors.New("execution materials unavailable or corrupt")

// Fetch prepares only the pinned M0 fixture supported by the execution module.
// It reauthorizes every read, uses at most 15 seconds in total (3 per request),
// and returns no partial data on failure. No cache or credential is written to
// disk. The caller retains the original lease and renews before execution.
func (a *Acquirer) Fetch(ctx context.Context, lease submission.Lease) (materials.Descriptor, map[string][]byte, error) {
	if !validLease(lease) {
		return materials.Descriptor{}, nil, submission.ErrLease
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	d, err := a.describe(ctx, lease)
	if err != nil {
		return materials.Descriptor{}, nil, err
	}
	// Parse only after equality with the canonical, supported manifest. Generic
	// packages and execution profiles must get their own admission path later.
	var m struct {
		Tests []struct {
			ID     string `json:"id"`
			Input  string `json:"input_sha256"`
			Answer string `json:"answer_sha256"`
		} `json:"tests"`
	}
	if json.Unmarshal(d.Manifest, &m) != nil || len(m.Tests) == 0 {
		return materials.Descriptor{}, nil, ErrMaterials
	}
	blobs := map[string][]byte{}
	total := 0
	for _, test := range m.Tests {
		for _, item := range []struct{ kind, hash string }{{"input", test.Input}, {"answer", test.Answer}} {
			data, err := a.download(ctx, lease.Token, test.ID, item.kind, item.hash)
			if err != nil {
				return materials.Descriptor{}, nil, err
			}
			total += len(data)
			if total > 4<<20 {
				return materials.Descriptor{}, nil, ErrMaterials
			}
			blobs[item.hash] = data
		}
	}
	if ctx.Err() != nil {
		return materials.Descriptor{}, nil, ErrMaterials
	}
	return d, blobs, nil
}

func (a *Acquirer) describe(ctx context.Context, lease submission.Lease) (materials.Descriptor, error) {
	resp, err := a.post(ctx, "assignment", struct {
		Token string `json:"lease_token"`
	}{lease.Token})
	if err != nil {
		return materials.Descriptor{}, ErrMaterials
	}
	defer resp.Body.Close()
	if err := materialStatus(resp.StatusCode); err != nil {
		return materials.Descriptor{}, err
	}
	var d materials.Descriptor
	// JSON escaping can expand a valid source byte sixfold; the decoded limits
	// below remain authoritative, including base64 canonical manifest bytes.
	if readJSON(resp, 6*submission.MaxSourceBytes+32768, &d) != nil {
		return materials.Descriptor{}, ErrMaterials
	}
	_, fixed, err := experiment.SelectFixture(experiment.PinnedVersion)
	if err != nil || d.SubmissionID != lease.SubmissionID || d.Version != experiment.PinnedVersion ||
		d.Language != "gnu++17" || !bytes.Equal(d.Manifest, fixed) || checksum(d.Manifest) != d.ManifestSHA256 ||
		len(d.Source) == 0 || len(d.Source) > submission.MaxSourceBytes || !utf8.ValidString(d.Source) || checksum([]byte(d.Source)) != d.SourceSHA256 {
		return materials.Descriptor{}, ErrMaterials
	}
	return d, nil
}

func (a *Acquirer) download(ctx context.Context, token, id, kind, hash string) ([]byte, error) {
	resp, err := a.post(ctx, "material", struct {
		Token string `json:"lease_token"`
		ID    string `json:"test_id"`
		Kind  string `json:"kind"`
	}{token, id, kind})
	if err != nil {
		return nil, ErrMaterials
	}
	defer resp.Body.Close()
	if err := materialStatus(resp.StatusCode); err != nil {
		return nil, err
	}
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "application/octet-stream" || resp.Header.Get("X-Content-SHA256") != hash ||
		resp.ContentLength < 0 || resp.ContentLength > judging.TextBudget {
		return nil, ErrMaterials
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, judging.TextBudget+1))
	if err != nil || len(data) > judging.TextBudget || int64(len(data)) != resp.ContentLength || checksum(data) != hash {
		return nil, ErrMaterials
	}
	return data, nil
}

func materialStatus(status int) error {
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return submission.ErrNode
	case http.StatusConflict:
		return submission.ErrLease
	default:
		return ErrMaterials
	}
}

func checksum(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

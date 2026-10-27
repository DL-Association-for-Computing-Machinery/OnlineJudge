package materials

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

var testIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Descriptor gives the authenticated node source and the exact pinned manifest.
type Descriptor struct {
	submission.TaskMaterials
	SourceSHA256 string `json:"source_sha256"`
}

// Service couples task scope to immutable blob reads, without holding DB locks during I/O.
type Service struct {
	store  submission.MaterialStore
	reader Reader
}

// New binds an authorization store and a replaceable blob reader.
func New(store submission.MaterialStore, reader Reader) *Service {
	return &Service{store: store, reader: reader}
}

// Describe reauthorizes even if the worker has already cached these materials.
func (s *Service) Describe(ctx context.Context, node submission.NodeIdentity, token string) (Descriptor, error) {
	if s == nil || s.store == nil {
		return Descriptor{}, ErrUnavailable
	}
	value, err := s.store.Materials(ctx, node, token)
	if err != nil {
		return Descriptor{}, err
	}
	if len(value.Manifest) > 16384 || len(value.Source) > submission.MaxSourceBytes || checksum(value.Manifest) != value.ManifestSHA256 {
		return Descriptor{}, ErrUnavailable
	}
	if _, err := parseManifest(value); err != nil {
		return Descriptor{}, err
	}
	return Descriptor{TaskMaterials: value, SourceSHA256: checksum([]byte(value.Source))}, nil
}

// Download permits only a test/kind present in this lease's pinned manifest.
// Authorization is checked again after potentially slow disk I/O, before any bytes escape.
func (s *Service) Download(ctx context.Context, node submission.NodeIdentity, token, testID, kind string) (*File, error) {
	if !testIDPattern.MatchString(testID) || (kind != "input" && kind != "answer") {
		return nil, submission.ErrInvalid
	}
	first, err := s.Describe(ctx, node, token)
	if err != nil {
		return nil, err
	}
	manifest, err := parseManifest(first.TaskMaterials)
	if err != nil {
		return nil, err
	}
	hash := ""
	for _, test := range manifest.Tests {
		if test.ID == testID {
			if kind == "input" {
				hash = test.Input
			} else {
				hash = test.Answer
			}
			break
		}
	}
	if hash == "" {
		return nil, submission.ErrNotFound
	}
	if s.reader == nil {
		return nil, ErrUnavailable
	}
	file, err := s.reader.Open(ctx, hash)
	if err != nil {
		return nil, err
	}
	current, err := s.Describe(ctx, node, token)
	if err != nil {
		file.Close()
		return nil, err
	}
	if current.SubmissionID != first.SubmissionID || current.ManifestSHA256 != first.ManifestSHA256 || current.SourceSHA256 != first.SourceSHA256 {
		file.Close()
		return nil, ErrUnavailable
	}
	return file, nil
}

type test struct {
	ID     string `json:"id"`
	Input  string `json:"input_sha256"`
	Answer string `json:"answer_sha256"`
}
type manifest struct {
	Version  string `json:"version"`
	Language string `json:"language"`
	Tests    []test `json:"tests"`
}

func parseManifest(value submission.TaskMaterials) (manifest, error) {
	var m manifest
	if json.Unmarshal(value.Manifest, &m) != nil || m.Version != value.Version || m.Language != value.Language || len(m.Tests) == 0 || len(m.Tests) > 64 {
		return m, ErrUnavailable
	}
	seen := map[string]bool{}
	for _, test := range m.Tests {
		if !testIDPattern.MatchString(test.ID) || seen[test.ID] || !digestPattern.MatchString(test.Input) || !digestPattern.MatchString(test.Answer) {
			return m, ErrUnavailable
		}
		seen[test.ID] = true
	}
	return m, nil
}
func checksum(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

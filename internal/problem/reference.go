package problem

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

// ErrReferenceUnsupported means the complete draft cannot be represented by the
// current reference-validation protocol. Never sample or drop unsupported points.
var ErrReferenceUnsupported = errors.New("draft reference validation unsupported")

// ReferenceLimits bounds the frozen plan before any program can execute.
type ReferenceLimits struct {
	SourceBytes int64 `json:"source_bytes"`
	InputBytes  int64 `json:"input_bytes"`
	AnswerBytes int64 `json:"answer_bytes"`
	TotalBytes  int64 `json:"total_bytes"`
	Cases       int   `json:"cases"`
}

// Valid caps the small-material protocol independently of its caller's profile.
func (l ReferenceLimits) Valid() bool {
	return l.SourceBytes > 0 && l.SourceBytes <= 64<<10 && l.InputBytes > 0 && l.InputBytes <= 16<<10 && l.AnswerBytes > 0 && l.AnswerBytes <= 4096 && l.TotalBytes > 0 && l.TotalBytes <= 2<<20 && l.Cases > 0 && l.Cases <= 64
}

// ReferenceCase binds one ordered point to exact frozen private bytes.
type ReferenceCase struct {
	ID         string               `json:"id"`
	Input      materialstore.Object `json:"input"`
	Answer     materialstore.Object `json:"answer"`
	InputData  []byte               `json:"-"`
	AnswerData []byte               `json:"-"`
}

// ReferenceDependency binds a sandbox-relative name to frozen source bytes.
type ReferenceDependency struct {
	Path   string               `json:"path"`
	Object materialstore.Object `json:"object"`
}

// ReferenceSourceFile contains private compilation bytes, never report data.
type ReferenceSourceFile struct {
	Path string `json:"path"`
	Data []byte `json:"-"`
}

// ReferencePlan is a detached maintenance snapshot, not a permission grant or a
// published problem version. Its JSON contains identities, never private bytes.
type ReferencePlan struct {
	SourceID         string                `json:"source_id"`
	Version          string                `json:"version"`
	ImportRevision   string                `json:"import_revision"`
	SourceLockSHA256 string                `json:"source_lock_sha256"`
	Source           materialstore.Object  `json:"source"`
	SourcePath       string                `json:"source_path,omitempty"`
	Dependencies     []ReferenceDependency `json:"dependencies,omitempty"`
	Definition       materialstore.Object  `json:"test_definition"`
	Hashes           materialstore.Object  `json:"test_hashes"`
	Cases            []ReferenceCase       `json:"cases"`
	SourceData       []byte                `json:"-"`
}

// Digest binds source, draft/import identities and the complete ordered case
// list. It is SHA-256 of compact encoding/json bytes, without a trailing newline.
func (p ReferencePlan) Digest() string { raw, _ := json.Marshal(p); return hash(raw) }

// PrepareReference freezes ALL selected materials using the same opened file
// for bounded reads and hashing. Only the first fixed source layout is admitted;
// arbitrary includes, scripts and other source families require a new protocol.
// No program runs here. Callers supply the trusted source lock to LoadBundle.
func (p *Prepared) PrepareReference(ctx context.Context, limits ReferenceLimits) (ReferencePlan, error) {
	var plan ReferencePlan
	if p == nil || ctx == nil || !limits.Valid() {
		return plan, ErrInvalid
	}
	if err := p.acquire(); err != nil {
		return plan, err
	}
	defer p.release()
	plan, d, err := p.referenceMetadata(ctx, limits, "library-checker:aplusb", "sample/aplusb")
	if err != nil {
		return plan, err
	}
	plan.SourceData, err = p.readReference(ctx, d.Provenance.OriginalFiles["source/sample/aplusb/sol/correct.cpp"])
	if err != nil {
		return plan, err
	}
	for i, c := range d.TestCases {
		input, e := p.readReference(ctx, c.Input)
		if e != nil {
			return plan, e
		}
		answer, e := p.readReference(ctx, c.Answer)
		if e != nil {
			return plan, e
		}
		plan.Cases[i].InputData, plan.Cases[i].AnswerData = input, answer
	}
	return plan, ctx.Err()
}

func (p *Prepared) referenceMetadata(ctx context.Context, limits ReferenceLimits, sourceID, sourcePath string) (ReferencePlan, document, error) {
	var plan ReferencePlan
	d, err := parseDocument(p.content.Document, p.content.SourceID)
	if err != nil {
		return plan, d, err
	}
	plan = ReferencePlan{SourceID: p.content.SourceID, Version: p.content.Version, ImportRevision: p.content.RequestSHA256, SourceLockSHA256: d.Provenance.SourceLockSHA256}
	for _, c := range d.TestCases {
		plan.Cases = append(plan.Cases, ReferenceCase{ID: c.ID, Input: c.Input.object(), Answer: c.Answer.object()})
	}
	if d.Provenance.SourceID != sourceID || d.Provenance.SourcePath != sourcePath || len(d.TestCases) > limits.Cases {
		return plan, d, ErrReferenceUnsupported
	}
	source, ok := d.Provenance.OriginalFiles["source/"+sourcePath+"/sol/correct.cpp"]
	if !ok || source.Size < 1 || source.Size > limits.SourceBytes {
		return plan, d, ErrReferenceUnsupported
	}
	definition, hasDefinition := d.Provenance.OriginalFiles["source/"+sourcePath+"/info.toml"]
	hashes, hasHashes := d.Provenance.OriginalFiles["source/"+sourcePath+"/hash.json"]
	if !hasDefinition || !hasHashes || definition.Size < 1 || hashes.Size < 1 || definition.Size > maxReferenceCatalog || hashes.Size > maxReferenceCatalog {
		return plan, d, ErrReferenceUnsupported
	}
	plan.Source, plan.Definition, plan.Hashes = source.object(), definition.object(), hashes.object()
	total := source.Size + definition.Size + hashes.Size
	if total > limits.TotalBytes {
		return plan, d, ErrReferenceUnsupported
	}
	for _, c := range d.TestCases {
		if c.Input.Size > limits.InputBytes || c.Answer.Size > limits.AnswerBytes {
			return plan, d, ErrReferenceUnsupported
		}
		for _, size := range []int64{c.Input.Size, c.Answer.Size} {
			if size > limits.TotalBytes-total {
				return plan, d, ErrReferenceUnsupported
			}
			total += size
		}
	}
	info, err := p.readReference(ctx, definition)
	if err != nil {
		return plan, d, err
	}
	hashList, err := p.readReference(ctx, hashes)
	if err != nil {
		return plan, d, err
	}
	if err := validateReferenceCatalog(d, info, hashList, limits.Cases); err != nil {
		return plan, d, err
	}
	return plan, d, ctx.Err()
}

func (p *Prepared) readReference(ctx context.Context, ref fileReference) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := p.openFile(ref.Path, ref.Size)
	if err != nil {
		return nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, ref.Size+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || int64(len(raw)) != ref.Size || hash(raw) != ref.SHA256 {
		return nil, materialstore.ErrIntegrity
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return raw, nil
}

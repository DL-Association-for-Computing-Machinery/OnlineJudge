package problem

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	maximum := SmallReferenceLimits()
	return l.SourceBytes > 0 && l.SourceBytes <= maximum.SourceBytes && l.InputBytes > 0 && l.InputBytes <= maximum.InputBytes &&
		l.AnswerBytes > 0 && l.AnswerBytes <= maximum.AnswerBytes && l.TotalBytes > 0 && l.TotalBytes <= maximum.TotalBytes &&
		l.Cases > 0 && l.Cases <= maximum.Cases
}

// SmallReferenceLimits bounds the legacy A+B materials and its persisted equivalent.
func SmallReferenceLimits() ReferenceLimits {
	return ReferenceLimits{SourceBytes: 64 << 10, InputBytes: 16 << 10, AnswerBytes: 4096, TotalBytes: 2 << 20, Cases: 64}
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
	LocalText        bool                  `json:"local_text,omitempty"`
	Dependencies     []ReferenceDependency `json:"dependencies,omitempty"`
	Definition       materialstore.Object  `json:"test_definition"`
	Hashes           materialstore.Object  `json:"test_hashes"`
	Cases            []ReferenceCase       `json:"cases"`
	SourceData       []byte                `json:"-"`
}

type referenceOpen func(context.Context, fileReference) (io.ReadCloser, error)

func (p *Prepared) openReference(_ context.Context, ref fileReference) (io.ReadCloser, error) {
	f, err := p.openFile(ref.Path, ref.Size)
	if err != nil {
		return nil, err
	}
	return f, nil
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
	plan, d, err := referenceMetadata(p.content, limits, "library-checker:aplusb", "sample/aplusb")
	if err != nil {
		return plan, err
	}
	open := referenceOpen(p.openReference)
	if err := open.validateCatalog(ctx, d, limits.Cases); err != nil {
		return plan, err
	}
	plan.SourceData, err = open.read(ctx, d.Provenance.OriginalFiles["source/sample/aplusb/sol/correct.cpp"])
	if err != nil {
		return plan, err
	}
	for i, c := range d.TestCases {
		input, e := open.read(ctx, c.Input)
		if e != nil {
			return plan, e
		}
		answer, e := open.read(ctx, c.Answer)
		if e != nil {
			return plan, e
		}
		plan.Cases[i].InputData, plan.Cases[i].AnswerData = input, answer
	}
	return plan, ctx.Err()
}

func referenceMetadata(content Content, limits ReferenceLimits, sourceID, sourcePath string) (ReferencePlan, document, error) {
	var plan ReferencePlan
	d, err := parseDocument(content.Document, content.SourceID)
	if err != nil {
		return plan, d, err
	}
	plan = ReferencePlan{SourceID: content.SourceID, Version: content.Version, ImportRevision: content.RequestSHA256, SourceLockSHA256: d.Provenance.SourceLockSHA256}
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
	return plan, d, nil
}

func (open referenceOpen) validateCatalog(ctx context.Context, d document, maximum int) error {
	prefix := "source/" + d.Provenance.SourcePath + "/"
	info, err := open.read(ctx, d.Provenance.OriginalFiles[prefix+"info.toml"])
	if err != nil {
		return err
	}
	hashList, err := open.read(ctx, d.Provenance.OriginalFiles[prefix+"hash.json"])
	if err != nil {
		return err
	}
	return validateReferenceCatalog(d, info, hashList, maximum)
}

func (open referenceOpen) read(ctx context.Context, ref fileReference) ([]byte, error) {
	raw := bytes.NewBuffer(make([]byte, 0, int(ref.Size)))
	if err := open.copy(ctx, raw, ref); err != nil {
		return nil, err
	}
	return raw.Bytes(), nil
}

func (open referenceOpen) copy(ctx context.Context, dst io.Writer, ref fileReference) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := open(ctx, ref)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(dst, h), io.LimitReader(referenceContextReader{ctx, f}, ref.Size+1))
	closeErr := f.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	if copyErr != nil || closeErr != nil {
		return errors.New("reference snapshot copy failed")
	}
	if n != ref.Size || hex.EncodeToString(h.Sum(nil)) != ref.SHA256 {
		return materialstore.ErrIntegrity
	}
	return nil
}

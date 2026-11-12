package problem

import (
	"bytes"
	"slices"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// ImportIdentity binds the complete configured import and its trusted source
// lock. It says nothing about material health or successful program execution.
type ImportIdentity struct {
	SourceID         string
	Version          string
	ImportRevision   string
	SourceLockSHA256 string
}

// ImportIdentity reads only metadata frozen by LoadBundle, never material files.
func (p *Prepared) ImportIdentity() (ImportIdentity, error) {
	if p == nil {
		return ImportIdentity{}, ErrInvalid
	}
	d, err := parseDocument(p.content.Document, p.content.SourceID)
	if err != nil || d.Version != p.content.Version || !ValidImportRevision(p.content.RequestSHA256) {
		return ImportIdentity{}, ErrInvalid
	}
	return ImportIdentity{SourceID: p.content.SourceID, Version: p.content.Version,
		ImportRevision: p.content.RequestSHA256, SourceLockSHA256: d.Provenance.SourceLockSHA256}, nil
}

// DraftIdentity is the narrow owner-scoped identity locked by a business
// transaction, without returning documents or material/download references.
type DraftIdentity struct {
	ID, Owner, SourceID, Version, ImportRevision string
}

// BindReferenceContent matches a detached persisted draft to the exact import
// selected by the trusted catalog when the request was accepted. It checks
// metadata only; callers still need current authority and verified object bytes.
func (d Draft) BindReferenceContent(id, owner string, expected ImportIdentity) (Content, error) {
	doc, err := parseDocument(d.Document, d.SourceID)
	if err != nil {
		return Content{}, ErrInvalid
	}
	return d.bindReferenceContent(id, owner, expected, doc)
}

// MaterialInventoryIdentity verifies a restored draft's document and complete
// reference inventory, and returns its historical import identity. It grants no access.
func (d Draft) MaterialInventoryIdentity() (ImportIdentity, error) {
	doc, err := parseDocument(d.Document, d.SourceID)
	if err != nil {
		return ImportIdentity{}, err
	}
	i := ImportIdentity{SourceID: d.SourceID, Version: d.Version, ImportRevision: d.ImportRevision, SourceLockSHA256: doc.Provenance.SourceLockSHA256}
	if _, err := d.bindReferenceContent(d.ID, d.Owner, i, doc); err != nil {
		return ImportIdentity{}, err
	}
	return i, nil
}

func (d Draft) bindReferenceContent(id, owner string, expected ImportIdentity, doc document) (Content, error) {
	if !identity.ValidID(id) || !identity.ValidID(owner) || d.ID != id || d.Owner != owner ||
		d.State != "private-draft" || d.ValidationState != "not-run" ||
		d.SourceID != expected.SourceID || d.Version != expected.Version || d.ImportRevision != expected.ImportRevision ||
		len(d.References) == 0 || len(d.References) > 8192 {
		return Content{}, ErrInvalid
	}
	if doc.Version != expected.Version || doc.Provenance.SourceLockSHA256 != expected.SourceLockSHA256 {
		return Content{}, ErrInvalid
	}
	// PostgreSQL ordering can depend on collation; the original import used
	// Go's byte order. Sort a copy so the caller keeps its own snapshot intact.
	refs := slices.Clone(d.References)
	slices.SortFunc(refs, func(a, b Reference) int { return strings.Compare(a.Name, b.Name) })
	for i, ref := range refs {
		if !validPath(ref.Name) || !validHash(ref.Object.SHA256) || ref.Object.Size < 0 || ref.Object.Size > maxObject ||
			(i > 0 && refs[i-1].Name == ref.Name) {
			return Content{}, ErrInvalid
		}
	}
	revision, err := importRevision(d.Document, refs)
	if err != nil || revision != expected.ImportRevision {
		return Content{}, ErrInvalid
	}
	return Content{SourceID: d.SourceID, Version: d.Version, RequestSHA256: revision,
		Document: bytes.Clone(d.Document), References: refs}, nil
}

// Both import and rebinding hash the complete references in bytewise name order.
func importRevision(raw []byte, refs []Reference) (string, error) {
	request, err := canonical(struct {
		DocumentSHA256 string      `json:"document_sha256"`
		References     []Reference `json:"references"`
	}{hash(raw), refs})
	if err != nil {
		return "", err
	}
	return hash(request), nil
}

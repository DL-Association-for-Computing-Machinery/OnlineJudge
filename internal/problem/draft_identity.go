package problem

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

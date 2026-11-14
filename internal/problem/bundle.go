package problem

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

type bundleIndex struct {
	Schema            string                  `json:"schema"`
	State             string                  `json:"state"`
	Access            string                  `json:"access"`
	CopiedSourceBytes int64                   `json:"copied_source_bytes"`
	Files             map[string]fileIdentity `json:"files"`
	Problems          []struct {
		Draft     string `json:"draft"`
		Examples  int    `json:"examples"`
		SourceID  string `json:"source_id"`
		TestCases int    `json:"test_cases"`
		Version   string `json:"version"`
	} `json:"problems"`
}
type sourceLock struct {
	Schema     string `json:"schema"`
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Problems   []struct {
		Path  string `json:"path"`
		Title string `json:"title_zh"`
	} `json:"problems"`
	Files map[string]fileIdentity `json:"files"`
}
type preparedFile struct {
	ref      Reference
	path     string
	metadata []byte
}

// Prepared pins a trusted stable private bundle, snapshots small metadata and
// carries validated immutable references. It does not execute source programs.
// Ancestors and bundle contents must remain controlled by trusted maintenance;
// this is not an upload parser or isolation against a malicious same-UID writer.
type Prepared struct {
	root    *os.Root
	content Content
	files   []preparedFile
	mu      sync.Mutex
	active  bool
	closed  bool
}

// Content returns detached metadata, never an authorization grant.
func (p *Prepared) Content() Content {
	if p == nil {
		return Content{}
	}
	c := p.content
	c.Document = bytes.Clone(c.Document)
	c.References = append([]Reference(nil), c.References...)
	return c
}

// Close refuses active imports. Successful repeated Close is harmless.
func (p *Prepared) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active {
		return ErrBusy
	}
	if p.closed {
		return nil
	}
	p.closed = true
	return p.root.Close()
}
func (p *Prepared) acquire() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrInvalid
	}
	if p.active {
		return ErrBusy
	}
	p.active = true
	return nil
}
func (p *Prepared) release() { p.mu.Lock(); p.active = false; p.mu.Unlock() }

// LoadBundle selects one draft from a complete bundle. expectedSourceLock must
// be the trusted configured lock's original bytes, not bytes supplied by an
// untrusted requester. The bundle's index is integrity metadata, not a signature.
// Only bounded metadata is read here; large payloads are hashed by Put after
// Service author preflight. All references of the selected draft are persisted.
func LoadBundle(directory, sourceID string, expectedSourceLock []byte) (*Prepared, error) {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || len(expectedSourceLock) == 0 || len(expectedSourceLock) > maxMetadata {
		return nil, ErrInvalid
	}
	info, err := os.Lstat(directory)
	if err != nil || !privateInfo(info, true) {
		return nil, ErrInvalid
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, ErrInvalid
	}
	p := &Prepared{root: root}
	ok := false
	defer func() {
		if !ok {
			_ = root.Close()
		}
	}()
	// Both producer output forms may carry the marker. Inspect the trusted parent
	// explicitly; no parent file is read or copied.
	if _, err = os.Lstat(filepath.Join(filepath.Dir(directory), "INCOMPLETE")); !errors.Is(err, os.ErrNotExist) {
		return nil, ErrInvalid
	}
	if _, err = root.Lstat("INCOMPLETE"); !errors.Is(err, os.ErrNotExist) {
		return nil, ErrInvalid
	}
	indexRaw, err := p.readMetadata("index.json", nil)
	if err != nil {
		return nil, err
	}
	var index bundleIndex
	if _, err = canonicalMap(indexRaw); err != nil {
		return nil, err
	}
	if err = strictJSON(indexRaw, &index); err != nil || index.Schema != "oj.private-draft-bundle.v1" || index.State != "private-unpublished" || len(index.Files) < 1 || len(index.Files) > 8192 || len(index.Problems) < 1 || len(index.Problems) > 100 || index.CopiedSourceBytes < 0 || index.CopiedSourceBytes > maxBundle {
		return nil, ErrInvalid
	}
	if err = p.checkInventory(index.Files); err != nil {
		return nil, err
	}
	lockID, exists := index.Files["provenance/source-lock.json"]
	if !exists || lockID.SHA256 != hash(expectedSourceLock) || lockID.Size != int64(len(expectedSourceLock)) {
		return nil, ErrInvalid
	}
	lockRaw, err := p.readMetadata("provenance/source-lock.json", &lockID)
	if err != nil || !bytes.Equal(lockRaw, expectedSourceLock) {
		return nil, ErrInvalid
	}
	var lock sourceLock
	if err = strictJSON(lockRaw, &lock); err != nil || lock.Schema != "oj.library-checker-source.v1" || len(lock.Files) < 1 || len(lock.Files) > 4096 {
		return nil, ErrInvalid
	}
	var selected string
	seenIDs := map[string]bool{}
	seenDrafts := map[string]bool{}
	for _, entry := range index.Problems {
		if !validPath(entry.Draft) || !strings.HasPrefix(entry.Draft, "drafts/") || !strings.HasSuffix(entry.Draft, ".json") || seenIDs[entry.SourceID] || seenDrafts[entry.Draft] || !validHash(entry.Version) {
			return nil, ErrInvalid
		}
		seenIDs[entry.SourceID] = true
		seenDrafts[entry.Draft] = true
		identity, present := index.Files[entry.Draft]
		if !present {
			return nil, ErrInvalid
		}
		raw, e := p.readMetadata(entry.Draft, &identity)
		if e != nil {
			return nil, e
		}
		d, e := parseDocument(raw, entry.SourceID)
		if e != nil || d.Provenance.SourceLockSHA256 != lockID.SHA256 || d.Version != entry.Version || len(d.TestCases) != entry.TestCases || len(d.Examples) != entry.Examples {
			return nil, ErrInvalid
		}
		if entry.SourceID == sourceID {
			if err = p.selectDraft(d, raw, entry.Draft, index.Files, lock); err != nil {
				return nil, err
			}
			selected = entry.Draft
		}
	}
	if selected == "" {
		return nil, ErrNotFound
	}
	for name := range index.Files {
		if strings.HasPrefix(name, "drafts/") && !seenDrafts[name] {
			return nil, ErrInvalid
		}
	}
	for _, name := range []string{"provenance/source-lock.json", "provenance/source-catalog.json", "provenance/source-validation.json", "provenance/material-map.json"} {
		identity, present := index.Files[name]
		if !present {
			return nil, ErrInvalid
		}
		raw, e := p.readMetadata(name, &identity)
		if e != nil {
			return nil, e
		}
		p.files = append(p.files, preparedFile{ref: Reference{Name: name, Object: materialstore.Object{SHA256: identity.SHA256, Size: identity.Size}}, path: name, metadata: raw})
	}
	p.files = append(p.files, preparedFile{ref: Reference{Name: "index.json", Object: materialstore.Object{SHA256: hash(indexRaw), Size: int64(len(indexRaw))}}, path: "index.json", metadata: indexRaw})
	sort.Slice(p.files, func(i, j int) bool { return p.files[i].ref.Name < p.files[j].ref.Name })
	for _, f := range p.files {
		p.content.References = append(p.content.References, f.ref)
	}
	revision, err := importRevision(p.content.Document, p.content.References)
	if err != nil {
		return nil, err
	}
	p.content.RequestSHA256 = revision
	ok = true
	return p, nil
}

func (p *Prepared) selectDraft(d document, raw []byte, draftPath string, files map[string]fileIdentity, lock sourceLock) error {
	if d.Provenance.Commit != lock.Commit || d.Provenance.Repository != lock.Repository {
		return ErrInvalid
	}
	matched := false
	for _, item := range lock.Problems {
		if item.Path == d.Provenance.SourcePath && item.Title == d.Title {
			matched = true
		}
	}
	if !matched {
		return ErrInvalid
	}
	expected := map[string]fileIdentity{}
	for name, id := range lock.Files {
		if !validPath(name) || !validHash(id.SHA256) || id.Size < 0 || id.Size > maxObject {
			return ErrInvalid
		}
		if name == "LICENSE" || strings.HasPrefix(name, "common/") || name == "generate.py" || strings.HasPrefix(name, d.Provenance.SourcePath+"/") {
			expected["source/"+name] = id
		}
	}
	if len(expected) != len(d.Provenance.OriginalFiles) {
		return ErrInvalid
	}
	for name, id := range expected {
		if ref, present := d.Provenance.OriginalFiles[name]; !present || ref.identity() != id {
			return ErrInvalid
		}
	}
	refs := map[string]fileReference{"statement": d.Statement.fileReference}
	for _, c := range d.TestCases {
		refs["tests/"+c.ID+"/input"] = c.Input
		refs["tests/"+c.ID+"/answer"] = c.Answer
	}
	for name, r := range d.Provenance.OriginalFiles {
		refs["original/"+name] = r
	}
	for name, r := range refs {
		if !validPath(name) || files[r.Path] != r.identity() {
			return ErrInvalid
		}
		p.files = append(p.files, preparedFile{ref: Reference{Name: name, Object: r.object()}, path: r.Path})
	}
	p.content = Content{SourceID: d.Provenance.SourceID, Version: d.Version, Document: bytes.Clone(raw)}
	p.files = append(p.files, preparedFile{ref: Reference{Name: "draft.json", Object: materialstore.Object{SHA256: hash(raw), Size: int64(len(raw))}}, path: draftPath, metadata: bytes.Clone(raw)})
	return nil
}

func (p *Prepared) persist(ctx context.Context, store materialstore.Store) error {
	for _, item := range p.files {
		if err := ctx.Err(); err != nil {
			return err
		}
		var reader io.Reader
		var file *os.File
		if item.metadata != nil {
			reader = bytes.NewReader(item.metadata)
		} else {
			var err error
			file, err = p.openFile(item.path, item.ref.Object.Size)
			if err != nil {
				return err
			}
			reader = file
		}
		got, err := store.Put(ctx, item.ref.Object, reader)
		if file != nil {
			err = errors.Join(err, file.Close())
		}
		if err != nil {
			return err
		}
		if got != item.ref.Object {
			return materialstore.ErrIntegrity
		}
	}
	return nil
}

// ReadLocalTextQualityFile reads a small original, authenticated by the
// configured source lock. Only package quality files are exposed here.
func (p *Prepared) ReadLocalTextQualityFile(ctx context.Context, name string, maxBytes int64) ([]byte, error) {
	if p == nil || ctx == nil || !validPath(name) || !strings.HasPrefix(name, "quality/") ||
		maxBytes < 1 || maxBytes > maxReferenceCatalog {
		return nil, ErrInvalid
	}
	d, err := parseDocument(p.content.Document, p.content.SourceID)
	if err != nil {
		return nil, err
	}
	ref, ok := d.Provenance.OriginalFiles["source/"+d.Provenance.SourcePath+"/"+name]
	if !ok || ref.Size < 1 || ref.Size > maxBytes {
		return nil, ErrReferenceUnsupported
	}
	// Verified bundle metadata is immutable. The caller owns the bundle and
	// serializes access with Close, including while holding the formal snapshot.
	open := referenceOpen(p.openReference)
	return open.read(ctx, ref)
}

// LocalTextDisplayBinding pins the exact statement, images and ordered samples
// through the immutable import identity. Sample positions come from the draft.
func (p *Prepared) LocalTextDisplayBinding() (string, []bool, error) {
	id, err := p.ImportIdentity()
	if err != nil {
		return "", nil, err
	}
	d, err := parseDocument(p.content.Document, p.content.SourceID)
	if err != nil {
		return "", nil, err
	}
	samples := make([]bool, len(d.TestCases))
	for i, c := range d.TestCases {
		samples[i] = c.Sample
	}
	return id.ImportRevision, samples, nil
}

func privateInfo(info fs.FileInfo, directory bool) bool {
	if info == nil {
		return false
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Geteuid()) {
		return false
	}
	if directory {
		return info.Mode() == os.ModeDir|0700
	}
	return info.Mode() == 0600
}

func (p *Prepared) openFile(name string, size int64) (*os.File, error) {
	if !validPath(name) || size < 0 || size > maxObject {
		return nil, ErrInvalid
	}
	// Stable trusted source is required; explicit Lstat rejects directory links
	// as well as final links. OpenRoot confines resolution; NOFOLLOW/NONBLOCK
	// prevent final symlink and FIFO blocking even if the snapshot changes.
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		info, err := p.root.Lstat(parent)
		if err != nil || !privateInfo(info, true) {
			return nil, ErrInvalid
		}
	}
	f, err := p.root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrInvalid
	}
	info, err := f.Stat()
	if err != nil || !privateInfo(info, false) || info.Size() != size {
		_ = f.Close()
		return nil, ErrInvalid
	}
	return f, nil
}

func (p *Prepared) readMetadata(name string, expected *fileIdentity) ([]byte, error) {
	info, err := p.root.Lstat(name)
	if err != nil || !privateInfo(info, false) || info.Size() > maxMetadata {
		return nil, ErrInvalid
	}
	f, err := p.openFile(name, info.Size())
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxMetadata+1))
	closeErr := f.Close()
	if err != nil || closeErr != nil || int64(len(raw)) != info.Size() {
		return nil, ErrInvalid
	}
	if expected != nil && (expected.Size != int64(len(raw)) || expected.SHA256 != hash(raw)) {
		return nil, ErrInvalid
	}
	return raw, nil
}

func (p *Prepared) checkInventory(files map[string]fileIdentity) error {
	var total int64
	found := map[string]bool{}
	for name, id := range files {
		if !validPath(name) || name == "index.json" || !validHash(id.SHA256) || id.Size < 0 || id.Size > maxObject {
			return ErrInvalid
		}
		total += id.Size
		if total > maxBundle {
			return ErrInvalid
		}
	}
	err := fs.WalkDir(p.root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return ErrInvalid
		}
		info, err := entry.Info()
		if err != nil || !privateInfo(info, entry.IsDir()) {
			return ErrInvalid
		}
		if entry.IsDir() {
			return nil
		}
		if name == "index.json" {
			return nil
		}
		id, present := files[name]
		if !present || info.Size() != id.Size {
			return ErrInvalid
		}
		found[name] = true
		return nil
	})
	if err != nil || len(found) != len(files) {
		return ErrInvalid
	}
	return nil
}

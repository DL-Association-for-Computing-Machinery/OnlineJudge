package problem

import (
	"context"
	"errors"
	"sort"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ErrUnknownSource is returned only after current session and author preflight.
var ErrUnknownSource = errors.New("private draft source is not configured")

// Importer selects from a frozen, trusted catalog after authorization. It owns
// no resources: its caller must drain requests before closing the preparations.
type Importer struct {
	service *Service
	sources map[string]*Prepared
	catalog []ImportSource
	slots   chan struct{}
}

// NewImporter copies a bounded catalog and requires explicit per-instance
// material concurrency shared by imports and previews. This is neither a shared
// rate limit nor a persistent disk quota.
func NewImporter(service *Service, sources map[string]*Prepared, maxInFlight int) (*Importer, error) {
	if service == nil || len(sources) == 0 || len(sources) > 10 || maxInFlight < 1 || maxInFlight > 10 {
		return nil, ErrInvalid
	}
	i := &Importer{service: service, sources: make(map[string]*Prepared, len(sources)), slots: make(chan struct{}, maxInFlight)}
	for id, p := range sources {
		content := p.Content()
		if p == nil || id == "" || len(id) > 128 || content.SourceID != id || !ValidImportRevision(content.RequestSHA256) {
			return nil, ErrInvalid
		}
		doc, err := parseDocument(content.Document, id)
		if err != nil || doc.Version != content.Version {
			return nil, ErrInvalid
		}
		i.sources[id] = p
		i.catalog = append(i.catalog, ImportSource{SourceID: id, Title: doc.Title, Version: content.Version, ImportRevision: content.RequestSHA256})
	}
	sort.Slice(i.catalog, func(a, b int) bool { return i.catalog[a].SourceID < i.catalog[b].SourceID })
	return i, nil
}

// Import never accepts a filesystem path. Unknown candidates and capacity are
// observed only by an authorized author, before any material I/O.
func (i *Importer) Import(ctx context.Context, credential redisstore.Credential, key, source string) (Draft, error) {
	return i.importSelected(ctx, credential, key, source, "", "")
}

func (i *Importer) importSelected(ctx context.Context, credential redisstore.Credential, key, source, expectedRevision, expectedSubject string) (Draft, error) {
	if i == nil || ctx == nil || !ValidKey(key) {
		return Draft{}, ErrInvalid
	}
	acquired := false
	defer func() {
		if acquired {
			<-i.slots
		}
	}()
	return i.service.create(ctx, credential, key, expectedSubject, func() (*Prepared, error) {
		p, ok := i.sources[source]
		if !ok {
			return nil, ErrUnknownSource
		}
		if expectedRevision != "" && p.content.RequestSHA256 != expectedRevision {
			return nil, ErrSourceChanged
		}
		if err := i.acquire(); err != nil {
			return nil, err
		}
		acquired = true
		return p, nil
	})
}

func (i *Importer) acquire() error {
	select {
	case i.slots <- struct{}{}:
		return nil
	default:
		return ErrBusy
	}
}

// AcquireMaterialSlot shares the draft runtime's per-instance material budget.
// The caller owns the returned release and must call it exactly once.
func (i *Importer) AcquireMaterialSlot() (func(), error) {
	if i == nil {
		return nil, ErrInvalid
	}
	if err := i.acquire(); err != nil {
		return nil, err
	}
	return func() { <-i.slots }, nil
}

// Preview reads persisted owner-scoped statement/sample materials. It does not
// depend on the current import catalog containing the old draft version.
func (i *Importer) Preview(ctx context.Context, credential redisstore.Credential, id string) (Preview, error) {
	if i == nil || ctx == nil || !identity.ValidID(id) {
		return Preview{}, ErrInvalid
	}
	acquired := false
	defer func() {
		if acquired {
			<-i.slots
		}
	}()
	return i.service.preview(ctx, credential, id, func() error {
		if err := i.acquire(); err != nil {
			return err
		}
		acquired = true
		return nil
	})
}

// GetOwn delegates to the same fresh session and transactional owner boundary.
func (i *Importer) GetOwn(ctx context.Context, credential redisstore.Credential, id string) (Draft, error) {
	return i.service.GetOwn(ctx, credential, id)
}

package problem

import (
	"context"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ErrUnknownSource is returned only after current session and author preflight.
var ErrUnknownSource = errors.New("private draft source is not configured")

// Importer selects from a frozen, trusted catalog after authorization. It owns
// no resources: its caller must drain requests before closing the preparations.
type Importer struct {
	service *Service
	sources map[string]*Prepared
	slots   chan struct{}
}

// NewImporter copies a bounded catalog and requires explicit per-instance
// concurrency. This is neither a shared rate limit nor a persistent disk quota.
func NewImporter(service *Service, sources map[string]*Prepared, maxInFlight int) (*Importer, error) {
	if service == nil || len(sources) == 0 || len(sources) > 10 || maxInFlight < 1 || maxInFlight > 10 {
		return nil, ErrInvalid
	}
	i := &Importer{service: service, sources: make(map[string]*Prepared, len(sources)), slots: make(chan struct{}, maxInFlight)}
	for id, p := range sources {
		if p == nil || id == "" || p.Content().SourceID != id {
			return nil, ErrInvalid
		}
		i.sources[id] = p
	}
	return i, nil
}

// Import never accepts a filesystem path. Unknown candidates and capacity are
// observed only by an authorized author, before any material I/O.
func (i *Importer) Import(ctx context.Context, credential redisstore.Credential, key, source string) (Draft, error) {
	if i == nil || ctx == nil || !ValidKey(key) {
		return Draft{}, ErrInvalid
	}
	acquired := false
	defer func() {
		if acquired {
			<-i.slots
		}
	}()
	return i.service.create(ctx, credential, key, func() (*Prepared, error) {
		p, ok := i.sources[source]
		if !ok {
			return nil, ErrUnknownSource
		}
		select {
		case i.slots <- struct{}{}:
			acquired = true
			return p, nil
		default:
			return nil, ErrBusy
		}
	})
}

// GetOwn delegates to the same fresh session and transactional owner boundary.
func (i *Importer) GetOwn(ctx context.Context, credential redisstore.Credential, id string) (Draft, error) {
	return i.service.GetOwn(ctx, credential, id)
}

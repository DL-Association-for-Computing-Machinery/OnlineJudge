package problem

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ErrSourceChanged means the configured full import revision differs from the
// caller's selection. It does not prove an earlier request never committed.
var ErrSourceChanged = errors.New("private draft import revision changed")

// ImportSource identifies a configured immutable candidate. Version identifies
// the problem document; ImportRevision also binds every evidence/reference.
type ImportSource struct {
	SourceID       string `json:"source_id"`
	Title          string `json:"title"`
	Version        string `json:"version"`
	ImportRevision string `json:"import_revision"`
}

// ImportReceipt is the owner's committed import summary, not material access.
// A missing receipt is only an observation, not proof no POST is in flight.
type ImportReceipt struct {
	DraftID         string    `json:"draft_id"`
	RequestKey      string    `json:"request_key"`
	SourceID        string    `json:"source_id"`
	Version         string    `json:"version"`
	ImportRevision  string    `json:"import_revision"`
	State           string    `json:"state"`
	ValidationState string    `json:"validation_state"`
	CreatedAt       time.Time `json:"created_at"`
}

func receipt(d Draft) ImportReceipt {
	return ImportReceipt{DraftID: d.ID, RequestKey: d.RequestKey, SourceID: d.SourceID, Version: d.Version,
		ImportRevision: d.ImportRevision, State: d.State, ValidationState: d.ValidationState, CreatedAt: d.CreatedAt}
}

// ValidImportRevision accepts the exact opaque full-candidate digest.
func ValidImportRevision(value string) bool { return validHash(value) }

// Sources reveals configured candidates only after current author admission,
// then fences the detached summary with fresh session and final PG permission.
// Metadata was admitted at startup; this is not a material health check.
func (i *Importer) Sources(ctx context.Context, credential redisstore.Credential) ([]ImportSource, error) {
	return i.sourcesForSubject(ctx, credential, "")
}

// SourcesForSubject requires the current session to belong to expectedSubject.
func (i *Importer) SourcesForSubject(ctx context.Context, credential redisstore.Credential, expectedSubject string) ([]ImportSource, error) {
	if !identity.ValidID(expectedSubject) {
		return nil, ErrInvalid
	}
	return i.sourcesForSubject(ctx, credential, expectedSubject)
}

func (i *Importer) sourcesForSubject(ctx context.Context, credential redisstore.Credential, expectedSubject string) ([]ImportSource, error) {
	if i == nil || ctx == nil {
		return nil, ErrInvalid
	}
	actor, err := i.service.actorForSubject(ctx, credential, expectedSubject)
	if err != nil {
		return nil, err
	}
	if err = i.service.repository.CheckAuthor(ctx, actor); err != nil {
		return nil, err
	}
	sources := append([]ImportSource(nil), i.catalog...)
	latest, err := i.service.actor(ctx, credential)
	if err != nil {
		return nil, err
	}
	if latest != actor {
		return nil, identity.ErrConflict
	}
	if err = i.service.repository.CheckAuthor(ctx, latest); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return sources, nil
}

// ImportPinned requires the full revision selected by the caller, not just the
// problem document version. The existing create transaction remains authoritative.
func (i *Importer) ImportPinned(ctx context.Context, credential redisstore.Credential, key, source, expectedRevision string) (ImportReceipt, error) {
	return i.importPinnedForSubject(ctx, credential, key, source, expectedRevision, "")
}

// ImportPinnedForSubject pins both the candidate revision and session subject.
func (i *Importer) ImportPinnedForSubject(ctx context.Context, credential redisstore.Credential, key, source, expectedRevision, expectedSubject string) (ImportReceipt, error) {
	if !identity.ValidID(expectedSubject) {
		return ImportReceipt{}, ErrInvalid
	}
	return i.importPinnedForSubject(ctx, credential, key, source, expectedRevision, expectedSubject)
}

func (i *Importer) importPinnedForSubject(ctx context.Context, credential redisstore.Credential, key, source, expectedRevision, expectedSubject string) (ImportReceipt, error) {
	if !ValidImportRevision(expectedRevision) {
		return ImportReceipt{}, ErrInvalid
	}
	draft, err := i.importSelected(ctx, credential, key, source, expectedRevision, expectedSubject)
	if err != nil {
		return ImportReceipt{}, err
	}
	return receipt(draft), nil
}

// Receipt observes the current owner's committed key without selecting a
// candidate, accessing materials, or acquiring import/preview capacity.
func (i *Importer) Receipt(ctx context.Context, credential redisstore.Credential, key string) (ImportReceipt, error) {
	return i.receiptForSubject(ctx, credential, key, "")
}

// ReceiptForSubject observes a receipt only for the expected session subject.
func (i *Importer) ReceiptForSubject(ctx context.Context, credential redisstore.Credential, key, expectedSubject string) (ImportReceipt, error) {
	if !identity.ValidID(expectedSubject) {
		return ImportReceipt{}, ErrInvalid
	}
	return i.receiptForSubject(ctx, credential, key, expectedSubject)
}

func (i *Importer) receiptForSubject(ctx context.Context, credential redisstore.Credential, key, expectedSubject string) (ImportReceipt, error) {
	if i == nil || ctx == nil || !ValidKey(key) {
		return ImportReceipt{}, ErrInvalid
	}
	actor, err := i.service.actorForSubject(ctx, credential, expectedSubject)
	if err != nil {
		return ImportReceipt{}, err
	}
	result, err := i.service.repository.Receipt(ctx, actor, key)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return ImportReceipt{}, err
	}
	return result, nil
}

package problem

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// OwnDraftPageSize bounds each owner read. One extra row determines continuation.
const OwnDraftPageSize = 20

// ErrInvalidCursor is a deterministic input refusal, not a storage failure.
var ErrInvalidCursor = errors.New("invalid private draft list cursor")

// ListPosition is a seek boundary, never an identity or access grant.
type ListPosition struct {
	CreatedAt time.Time
	ID        string
}

// Valid accepts exact PostgreSQL microsecond timestamps representable in JSON.
func (p ListPosition) Valid() bool {
	year := p.CreatedAt.UTC().Year()
	return identity.ValidID(p.ID) && year >= 1 && year <= 9999 && p.CreatedAt.Nanosecond()%1000 == 0
}

// Valid checks the bounded committed summary, not authorization or materials.
func (r ImportReceipt) Valid() bool {
	return (ListPosition{CreatedAt: r.CreatedAt, ID: r.DraftID}).Valid() && ValidKey(r.RequestKey) &&
		len(r.SourceID) > 0 && len(r.SourceID) <= 128 && utf8.ValidString(r.SourceID) &&
		validHash(r.Version) && validHash(r.ImportRevision) && r.State == "private-draft" && r.ValidationState == "not-run"
}

// OwnDraftPage is one current committed view, not a snapshot across requests.
type OwnDraftPage struct {
	Drafts     []ImportReceipt `json:"drafts"`
	NextCursor string          `json:"next_cursor"`
}

func encodeListCursor(owner string, position ListPosition) string {
	raw := strings.Join([]string{"1", owner, strconv.FormatInt(position.CreatedAt.UnixMicro(), 10), position.ID}, ".")
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeListCursor(cursor, owner string) (*ListPosition, error) {
	if cursor == "" {
		return nil, nil
	}
	if len(cursor) > 192 {
		return nil, ErrInvalidCursor
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != cursor {
		return nil, ErrInvalidCursor
	}
	parts := strings.Split(string(raw), ".")
	if len(parts) != 4 || parts[0] != "1" || parts[1] != owner || !identity.ValidID(owner) {
		return nil, ErrInvalidCursor
	}
	micros, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || strconv.FormatInt(micros, 10) != parts[2] {
		return nil, ErrInvalidCursor
	}
	position := ListPosition{CreatedAt: time.UnixMicro(micros).UTC(), ID: parts[3]}
	if !position.Valid() {
		return nil, ErrInvalidCursor
	}
	return &position, nil
}

func olderThan(a, b ListPosition) bool {
	return a.CreatedAt.Before(b.CreatedAt) || a.CreatedAt.Equal(b.CreatedAt) && a.ID < b.ID
}

// ListOwnForSubject reads only the actual owner's metadata, independently of
// the configured candidates and material budget. Late commits may require a
// fresh first-page read; reaching the end never proves no import is in flight.
func (i *Importer) ListOwnForSubject(ctx context.Context, credential redisstore.Credential, cursor, expectedSubject string) (OwnDraftPage, error) {
	if i == nil || ctx == nil || !identity.ValidID(expectedSubject) {
		return OwnDraftPage{}, ErrInvalid
	}
	actor, err := i.service.actorForSubject(ctx, credential, expectedSubject)
	if err != nil {
		return OwnDraftPage{}, err
	}
	after, err := decodeListCursor(cursor, actor.SubjectID)
	if err != nil {
		return OwnDraftPage{}, err
	}
	rows, err := i.service.repository.ListOwn(ctx, actor, after)
	if err != nil {
		return OwnDraftPage{}, err
	}
	if err = ctx.Err(); err != nil {
		return OwnDraftPage{}, err
	}
	if len(rows) > OwnDraftPageSize+1 {
		return OwnDraftPage{}, ErrStore
	}
	page := OwnDraftPage{Drafts: make([]ImportReceipt, 0, min(len(rows), OwnDraftPageSize))}
	previous := after
	for n, row := range rows {
		row.CreatedAt = row.CreatedAt.UTC()
		position := ListPosition{CreatedAt: row.CreatedAt, ID: row.DraftID}
		if !row.Valid() || previous != nil && !olderThan(position, *previous) {
			return OwnDraftPage{}, ErrStore
		}
		if n < OwnDraftPageSize {
			page.Drafts = append(page.Drafts, row)
		}
		previous = &position
	}
	if len(rows) > OwnDraftPageSize {
		last := page.Drafts[OwnDraftPageSize-1]
		page.NextCursor = encodeListCursor(actor.SubjectID, ListPosition{CreatedAt: last.CreatedAt, ID: last.DraftID})
	}
	return page, nil
}

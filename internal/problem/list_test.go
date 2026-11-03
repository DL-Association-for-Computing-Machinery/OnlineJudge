package problem

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

type listRepository struct {
	*protocolRepository
	rows   []ImportReceipt
	err    error
	calls  int
	after  *ListPosition
	onList func()
}

func (r *listRepository) ListOwn(_ context.Context, actor identity.ProblemActor, after *ListPosition) ([]ImportReceipt, error) {
	r.calls++
	if actor.SubjectID != strings.Repeat("1", 32) {
		panic("list did not use actual actor")
	}
	r.after = after
	if r.onList != nil {
		r.onList()
	}
	return r.rows, r.err
}

func listRows(n int) []ImportReceipt {
	rows := make([]ImportReceipt, n)
	for j := range rows {
		rows[j] = ImportReceipt{DraftID: fmt.Sprintf("%032x", n-j), RequestKey: fmt.Sprintf("list-original-key-%04d", j), SourceID: "library-checker:aplusb", Version: strings.Repeat("b", 64), ImportRevision: strings.Repeat("c", 64), State: "private-draft", ValidationState: "not-run", CreatedAt: time.Date(2026, 11, 3, 0, 0, 0, 123456000, time.UTC)}
	}
	return rows
}

func TestOwnDraftListBoundedCurrentOwnerAndMaterialIndependence(t *testing.T) {
	for _, count := range []int{0, 20, 21} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			i, sessions, original, credential := protocolFixture(t, nil)
			repo := &listRepository{protocolRepository: original, rows: listRows(count)}
			i.service.repository = repo
			delete(i.sources, "library-checker:aplusb")
			i.slots <- struct{}{}
			page, err := i.ListOwnForSubject(t.Context(), credential, "", strings.Repeat("1", 32))
			<-i.slots
			if err != nil || page.Drafts == nil || len(page.Drafts) != min(count, 20) || (page.NextCursor != "") != (count > 20) || repo.calls != 1 || sessions.calls != 1 || original.materials.puts != 0 || original.checks != 0 {
				t.Fatal("list changed authority, capacity or page bounds", err)
			}
			if page.NextCursor != "" {
				position, err := decodeListCursor(page.NextCursor, strings.Repeat("1", 32))
				if err != nil || position.ID != repo.rows[19].DraftID || !position.CreatedAt.Equal(repo.rows[19].CreatedAt) {
					t.Fatal("cursor did not use last included row")
				}
				repo.rows = repo.rows[20:]
				second, err := i.ListOwnForSubject(t.Context(), credential, page.NextCursor, strings.Repeat("1", 32))
				if err != nil || len(second.Drafts) != 1 || second.NextCursor != "" || repo.after == nil || *repo.after != *position {
					t.Fatal("continuation did not preserve exact microseconds", err)
				}
			}
		})
	}
}

func TestOwnListSubjectAndCursorPrecedeRepository(t *testing.T) {
	owner := strings.Repeat("1", 32)
	position := ListPosition{CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 123456000, time.UTC), ID: strings.Repeat("a", 32)}
	valid := encodeListCursor(owner, position)
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for _, cursor := range []string{valid + "=", "\n" + valid, strings.Repeat("a", 193), enc("2." + owner + ".0." + position.ID), enc("1." + owner + ".+1." + position.ID), enc("1." + owner + ".01." + position.ID), enc("1." + owner + ".-0." + position.ID), enc("1." + owner + ".9223372036854775807." + position.ID), encodeListCursor(strings.Repeat("2", 32), position)} {
		i, _, original, credential := protocolFixture(t, nil)
		repo := &listRepository{protocolRepository: original}
		i.service.repository = repo
		page, err := i.ListOwnForSubject(t.Context(), credential, cursor, owner)
		if !errors.Is(err, ErrInvalidCursor) || repo.calls != 0 || !reflect.DeepEqual(page, OwnDraftPage{}) {
			t.Fatal("invalid/cross-owner cursor reached repository", err)
		}
	}
	i, _, original, credential := protocolFixture(t, nil)
	repo := &listRepository{protocolRepository: original}
	i.service.repository = repo
	_, err := i.ListOwnForSubject(t.Context(), credential, "not-a-cursor", strings.Repeat("a", 32))
	if !errors.Is(err, ErrSubjectChanged) || repo.calls != 0 {
		t.Fatal("header was not compared against actual session first", err)
	}
}

func TestOwnListNeverReturnsPartialOrMalformedRepositoryResults(t *testing.T) {
	for _, mode := range []string{"over-limit", "invalid-row", "duplicate", "ascending", "after-boundary", "submicro", "date-range", "forbidden", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			i, _, original, credential := protocolFixture(t, nil)
			repo := &listRepository{protocolRepository: original, rows: listRows(21)}
			i.service.repository = repo
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			cursor := ""
			want := ErrStore
			switch mode {
			case "over-limit":
				repo.rows = listRows(22)
			case "invalid-row":
				repo.rows[20].ImportRevision = "bad"
			case "duplicate":
				repo.rows[20] = repo.rows[19]
			case "ascending":
				repo.rows[20].CreatedAt = repo.rows[19].CreatedAt.Add(time.Microsecond)
			case "after-boundary":
				cursor = encodeListCursor(strings.Repeat("1", 32), ListPosition{CreatedAt: repo.rows[0].CreatedAt, ID: repo.rows[0].DraftID})
			case "submicro":
				repo.rows[20].CreatedAt = repo.rows[20].CreatedAt.Add(-time.Nanosecond)
			case "date-range":
				repo.rows[20].CreatedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
			case "forbidden":
				want = identity.ErrForbidden
				repo.err = want
			case "canceled":
				want = context.Canceled
				repo.onList = cancel
			}
			page, err := i.ListOwnForSubject(ctx, credential, cursor, strings.Repeat("1", 32))
			if !errors.Is(err, want) || !reflect.DeepEqual(page, OwnDraftPage{}) {
				t.Fatal("partial or invalid list published", err)
			}
		})
	}
}

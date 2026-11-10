package problemreference

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestBindDraftUsesFrozenImportWithoutAliasing(t *testing.T) {
	p := preparedCatalogFixture(t, problemfixture.New(t))
	expected := p.Content()
	catalog, err := NewCatalog(map[string]*problem.Prepared{expected.SourceID: p})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := catalog.Select(draftSelection(t, p))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	request := Request{DraftID: strings.Repeat("1", 32), Owner: strings.Repeat("2", 32), State: Queued, Specification: spec}
	newDraft := func() problem.Draft {
		c := p.Content()
		slices.Reverse(c.References)
		return problem.Draft{ID: request.DraftID, Owner: request.Owner, SourceID: c.SourceID, Version: c.Version,
			ImportRevision: c.RequestSHA256, State: "private-draft", ValidationState: "not-run", Document: c.Document, References: c.References}
	}
	draft := newDraft()
	bound, err := request.BindDraft(draft)
	if err != nil || !reflect.DeepEqual(bound, expected) {
		t.Fatal("could not recover the original import from reordered metadata", err)
	}
	bound.Document[0] = '!'
	bound.References[0].Name = "other"
	if !reflect.DeepEqual(draft, newDraft()) {
		t.Fatal("binding changed or shares caller-owned metadata")
	}
	for _, tc := range []struct {
		name   string
		change func(*problem.Draft, *Request)
	}{
		{"same content different draft", func(d *problem.Draft, _ *Request) { d.ID = strings.Repeat("3", 32) }},
		{"different owner", func(d *problem.Draft, _ *Request) { d.Owner = strings.Repeat("3", 32) }},
		{"different source lock", func(_ *problem.Draft, r *Request) {
			r.Specification.Identity.SourceLockSHA256 = strings.Repeat("a", 64)
		}},
		{"different revision", func(d *problem.Draft, _ *Request) { d.ImportRevision = strings.Repeat("a", 64) }},
		{"changed raw document", func(d *problem.Draft, _ *Request) { d.Document = append(d.Document, '\n') }},
		{"missing reference", func(d *problem.Draft, _ *Request) { d.References = d.References[1:] }},
		{"duplicate reference", func(d *problem.Draft, _ *Request) { d.References = append(d.References, d.References[0]) }},
		{"extra reference", func(d *problem.Draft, _ *Request) {
			extra := d.References[0]
			extra.Name = "extra"
			d.References = append(d.References, extra)
		}},
		{"changed evidence only", func(d *problem.Draft, _ *Request) {
			for i := range d.References {
				if d.References[i].Name == "provenance/source-validation.json" {
					d.References[i].Object.SHA256 = strings.Repeat("b", 64)
					return
				}
			}
			t.Fatal("missing fixture evidence")
		}},
		{"already validated draft", func(d *problem.Draft, _ *Request) { d.ValidationState = "passed" }},
		{"cancelled receipt", func(_ *problem.Draft, r *Request) { r.State = Cancelled }},
		{"corrupt profile digest", func(_ *problem.Draft, r *Request) { r.Specification.ProfileSHA256 = strings.Repeat("a", 64) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, r := newDraft(), request
			tc.change(&d, &r)
			got, err := r.BindDraft(d)
			if (!errors.Is(err, ErrInvalid) && !errors.Is(err, problem.ErrInvalid)) || !reflect.DeepEqual(got, problem.Content{}) {
				t.Fatal("binding did not reject changed metadata with a zero result", err)
			}
		})
	}
}
